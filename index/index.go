package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/didi/nightingale/v4/src/common/identity"
	"github.com/didi/nightingale/v4/src/common/loggeri"
	"github.com/didi/nightingale/v4/src/common/report"
	"github.com/didi/nightingale/v4/src/common/stats"

	"github.com/n9e/n9e-tsdb/index/cache"
	"github.com/n9e/n9e-tsdb/index/config"
	"github.com/n9e/n9e-tsdb/index/http"
	"github.com/n9e/n9e-tsdb/index/http/routes"
	"github.com/n9e/n9e-tsdb/index/rpc"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"
)

var (
	vers *bool
	help *bool
	conf *string

	version = "No Version Provided"
)

func init() {
	vers = flag.Bool("v", false, "display the version.")
	help = flag.Bool("h", false, "print this help.")
	conf = flag.String("f", "", "specify configuration file.")
	flag.Parse()

	if *vers {
		fmt.Println("Version:", version)
		os.Exit(0)
	}

	if *help {
		flag.Usage()
		os.Exit(0)
	}
}

func main() {
	aconf()
	pconf()
	start()

	cfg := config.Config

	loggeri.Init(cfg.Logger)
	go stats.Init("n9e.index")

	identity.Parse()
	cache.InitDB(cfg.Cache)

	go report.Init(cfg.Report)
	go rpc.Start()

	r := gin.New()
	routes.Config(r)
	http.Start(r, "index", cfg.Logger.Level)
	ending()
}

// auto detect configuration file
func aconf() {
	if *conf != "" && file.IsExist(*conf) {
		return
	}

	*conf = "etc/index.local.yml"
	if file.IsExist(*conf) {
		return
	}

	*conf = "etc/index.yml"
	if file.IsExist(*conf) {
		return
	}

	fmt.Println("no configuration file for index")
	os.Exit(1)
}

// parse configuration file
func pconf() {
	if err := config.Parse(*conf); err != nil {
		fmt.Println("cannot parse configuration file:", err)
		os.Exit(1)
	}
}

func ending() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	select {
	case <-c:
		fmt.Printf("stop signal caught, stopping... pid=%d\n", os.Getpid())
	}

	logger.Close()
	http.Shutdown()
	fmt.Println("sender stopped successfully")
}

func start() {
	runner.Init()
	fmt.Println("index start, use configuration file:", *conf)
	fmt.Println("runner.Cwd:", runner.Cwd)
	fmt.Println("runner.Hostname:", runner.Hostname)
}
