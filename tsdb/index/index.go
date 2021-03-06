package index

import (
	"fmt"
	"sync"
	"time"

	"github.com/didi/nightingale/v4/src/common/report"
	"github.com/didi/nightingale/v4/src/common/stats"
	"github.com/n9e/n9e-tsdb/tsdb/backend/rpc"

	"github.com/toolkits/pkg/logger"
)

var IndexList IndexAddrs

type IndexAddrs struct {
	sync.RWMutex
	Data []string
}

func (i *IndexAddrs) Set(addrs []string) {
	i.Lock()
	defer i.Unlock()
	i.Data = addrs
}

func (i *IndexAddrs) Get() []string {
	i.RLock()
	defer i.RUnlock()
	return i.Data
}

func GetIndexLoop() {
	t1 := time.NewTicker(time.Duration(9) * time.Second)
	GetIndex()
	for {
		<-t1.C
		GetIndex()
		addrs := rpc.ReNewPools(IndexList.Get())
		if len(addrs) > 0 {
			RebuildAllIndex(addrs) //addrs为新增的index实例列表，重新推一遍全量索引
		}
	}
}

func GetIndex() {
	instances, err := report.GetAlive("index")
	if err != nil {
		stats.Counter.Set("get.index.err", 1)
		logger.Warningf("get index list err:%v", err)
		return
	}

	activeIndexs := []string{}
	for _, instance := range instances {
		activeIndexs = append(activeIndexs, fmt.Sprintf("%s:%s", instance.Identity, instance.RPCPort))
	}

	IndexList.Set(activeIndexs)
	return
}
