package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/didi/nightingale/v4/src/common/identity"
	"github.com/didi/nightingale/v4/src/common/report"
	"github.com/didi/nightingale/v4/src/common/stats"
	"github.com/didi/nightingale/v4/src/models"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

type CacheSection struct {
	CacheDuration   int    `yaml:"cacheDuration"`
	CleanInterval   int    `yaml:"cleanInterval"`
	PersistInterval int    `yaml:"persistInterval"`
	PersistDir      string `yaml:"persistDir"`
	RebuildWorker   int    `yaml:"rebuildWorker"`
	MaxQueryCount   int    `yaml:"maxQueryCount"`
	ReportEndpoint  bool   `yaml:"reportEndpoint"`
	HbsMod          string `yaml:"hbsMod"`
}

var IndexDB *EndpointIndexMap
var NidIndexDB *EndpointIndexMap
var Config CacheSection
var NewEndpoints *list.SafeListLimited

var semaPermanence = semaphore.NewSemaphore(1)

func InitDB(cfg CacheSection) {
	Config = cfg

	IndexDB = &EndpointIndexMap{M: make(map[string]*MetricIndexMap)}
	NidIndexDB = &EndpointIndexMap{M: make(map[string]*MetricIndexMap)}
	NewEndpoints = list.NewSafeListLimited(100000)

	Rebuild(Config.PersistDir, Config.RebuildWorker)

	go StartCleaner(Config.CleanInterval, Config.CacheDuration)
	go StartPersist(Config.PersistInterval)

}

func StartCleaner(interval int, cacheDuration int) {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	for {
		<-ticker.C

		start := time.Now()
		IndexDB.Clean(int64(cacheDuration))
		NidIndexDB.Clean(int64(cacheDuration))
		logger.Infof("clean took %.2f ms\n", float64(time.Since(start).Nanoseconds())*1e-6)
	}
}

func StartPersist(interval int) {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	for {
		<-ticker.C

		if err := Persist("normal"); err != nil {
			logger.Errorf("persist error:%+v", err)
			stats.Counter.Set("persist.err", 1)
		}
	}
}

func Rebuild(persistenceDir string, concurrency int) {
	var dbDir string
	indexList := IndexList()
	if len(indexList) > 0 {
		err := getIndexFromRemote(indexList)
		if err == nil {
			dbDir = fmt.Sprintf("%s/%s", persistenceDir, "download")
		}
	}

	// dbDir ?????????????????????????????????????????????????????????
	if dbDir == "" {
		logger.Debug("rebuild index from local disk")
		dbDir = fmt.Sprintf("%s/%s", persistenceDir, "db")
	}

	endpointDir := dbDir + "/endpoint"
	nidDir := dbDir + "/nid"

	if err := RebuildFromDisk(IndexDB, endpointDir, concurrency); err != nil {
		logger.Warningf("rebuild index from local disk error:%+v", err)
	}

	if err := RebuildFromDisk(NidIndexDB, nidDir, concurrency); err != nil {
		logger.Warningf("rebuild index from local disk error:%+v", err)
	}
}

func RebuildFromDisk(indexDB *EndpointIndexMap, indexFileDir string, concurrency int) error {
	logger.Info("Try to rebuild index from disk")
	if !file.IsExist(indexFileDir) {
		return fmt.Errorf("index persistence dir [%s] don't exist", indexFileDir)
	}

	// ????????????
	files, err := ioutil.ReadDir(indexFileDir)
	if err != nil {
		return err
	}
	logger.Infof("There're [%d] endpoints need to rebuild", len(files))

	sema := semaphore.NewSemaphore(concurrency)
	for _, fileObj := range files {
		// ???????????????
		if fileObj.IsDir() {
			continue
		}
		endpoint := fileObj.Name()

		sema.Acquire()
		go func(endpoint string) {
			defer sema.Release()

			metricIndexMap, err := ReadIndexFromFile(indexFileDir, endpoint)
			if err != nil {
				logger.Errorf("read file error, [endpoint:%s][reason:%v]", endpoint, err)
				return
			}
			// ???????????????????????? endpoint ????????????????????? monapi
			if !metricIndexMap.IsReported() {
				NewEndpoints.PushFront(endpoint)
			}

			indexDB.Lock()
			indexDB.M[endpoint] = metricIndexMap
			indexDB.Unlock()
		}(endpoint)

	}
	logger.Infof("rebuild from disk done")
	return nil
}

func Persist(mode string) error {
	indexFileDir := Config.PersistDir

	switch mode {
	case "end":
		semaPermanence.Acquire()
		defer semaPermanence.Release()
	case "normal", "download":
		if !semaPermanence.TryAcquire() {
			return fmt.Errorf("permanence operate is already running")
		}
		defer semaPermanence.Release()
	default:
		return fmt.Errorf("wrong mode:%s", mode)
	}

	var tmpDir string
	if mode == "download" {
		tmpDir = fmt.Sprintf("%s/%s", indexFileDir, "download")
	} else {
		tmpDir = fmt.Sprintf("%s/%s", indexFileDir, "tmp")
	}

	// write endpoint index data to disk
	endpointDir := tmpDir + "/endpoint"
	if err := os.RemoveAll(endpointDir); err != nil {
		return err
	}
	// create tmp directory
	if err := os.MkdirAll(endpointDir, 0777); err != nil {
		return err
	}

	endpoints := IndexDB.GetEndpoints()
	epLength := len(endpoints)
	logger.Infof("save index data to disk[num:%d][mode:%s]\n", epLength, mode)

	for i, endpoint := range endpoints {
		logger.Infof("sync [%s] to disk, [%d%%] complete\n", endpoint, int((float64(i)/float64(epLength))*100))

		if err := WriteIndexToFile("endpoint", endpointDir, endpoint); err != nil {
			logger.Errorf("write %s index to file err:%v\n", endpoint, err)
		}
	}

	nidDir := tmpDir + "/nid"
	if err := os.RemoveAll(nidDir); err != nil {
		return err
	}
	// create tmp directory
	if err := os.MkdirAll(nidDir, 0777); err != nil {
		return err
	}

	nids := NidIndexDB.GetEndpoints()
	nidLength := len(nids)
	logger.Infof("save index data to disk[num:%d][mode:%s]\n", nidLength, mode)

	for i, nid := range nids {
		logger.Infof("sync [%s] to disk, [%d%%] complete\n", nid, int((float64(i)/float64(nidLength))*100))

		if err := WriteIndexToFile("nid", nidDir, nid); err != nil {
			logger.Errorf("write %s index to file err:%v\n", nid, err)
		}
	}

	logger.Info("finish syncing index data")

	if mode == "download" {
		idxPath := fmt.Sprintf("%s/%s", indexFileDir, "db.tar.gz")
		if err := TarGz(idxPath, tmpDir); err != nil {
			return err
		}
	}

	// clean up the discard directory
	oleIndexDir := fmt.Sprintf("%s/%s", indexFileDir, "db")
	if err := os.RemoveAll(oleIndexDir); err != nil {
		return err
	}

	// rename directory
	if err := os.Rename(tmpDir, oleIndexDir); err != nil {
		return err
	}

	return nil
}

func WriteIndexToFile(mod, indexDir, endpoint string) error {
	var metricIndexMap *MetricIndexMap
	var exists bool
	if mod == "endpoint" {
		metricIndexMap, exists = IndexDB.GetMetricIndexMap(endpoint)
		if !exists || metricIndexMap == nil {
			return fmt.Errorf("endpoint index doesn't found")
		}
	} else {
		metricIndexMap, exists = NidIndexDB.GetMetricIndexMap(endpoint)
		if !exists || metricIndexMap == nil {
			return fmt.Errorf("endpoint index doesn't found")
		}
	}

	metricIndexMap.RLock()
	body, err := json.Marshal(metricIndexMap)
	stats.Counter.Set("write.file", 1)
	metricIndexMap.RUnlock()

	if err != nil {
		return fmt.Errorf("marshal struct to json failed:%v", err)
	}

	err = ioutil.WriteFile(fmt.Sprintf("%s/%s", indexDir, endpoint), body, 0666)
	return err
}

func ReadIndexFromFile(indexDir, endpoint string) (*MetricIndexMap, error) {
	metricIndexMap := new(MetricIndexMap)

	body, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", indexDir, endpoint))
	if err != nil {
		return metricIndexMap, err
	}

	err = json.Unmarshal(body, metricIndexMap)
	return metricIndexMap, err
}

func IndexList() []*models.Instance {
	activeIndexes, err := report.GetAlive("index")
	if err != nil {
		return []*models.Instance{}
	}

	var instances []*models.Instance
	for _, instance := range activeIndexes {
		ident, _ := identity.GetIdent()
		if instance.Identity != ident {
			instances = append(instances, instance)
		}
	}
	return instances
}

func getIndexFromRemote(instances []*models.Instance) error {
	filepath := "db.tar.gz"
	request := func(idx int) error {
		url := fmt.Sprintf("http://%s:%s/api/index/idxfile", instances[idx].Identity, instances[idx].HTTPPort)
		resp, err := http.Get(url)
		if err != nil {
			logger.Warningf("get index from:%s err:%v", url, err)
			return err
		}
		defer resp.Body.Close()

		// Create the file
		out, err := os.Create(filepath)
		if err != nil {
			logger.Warningf("create file:%s err:%v", filepath, err)
			return err
		}
		defer out.Close()

		// Write the body to file
		_, err = io.Copy(out, resp.Body)
		if err != nil {
			logger.Warningf("io.Copy error:%+v", err)
			return err
		}
		return nil
	}

	perm := rand.Perm(len(instances))
	var err error
	// retry
	for i := range perm {
		err = request(perm[i])
		if err == nil {
			break
		}
	}

	if err != nil {
		return err
	}

	if err := UnTarGz(filepath, "."); err != nil {
		return err
	}

	return os.Remove(filepath)
}
