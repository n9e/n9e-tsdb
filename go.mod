module github.com/n9e/n9e-tsdb

go 1.14

require (
	github.com/codegangsta/negroni v1.0.0
	github.com/dgryski/go-tsz v0.0.0-20180227144327-03b7d791f4fe
	github.com/didi/nightingale/v4 v4.0.0
	github.com/gin-contrib/pprof v1.3.0
	github.com/gin-gonic/gin v1.7.0
	github.com/gorilla/mux v1.7.3
	github.com/mattn/go-isatty v0.0.12
	github.com/open-falcon/rrdlite v0.0.0-20200214140804-bf5829f786ad
	github.com/spf13/viper v1.7.1
	github.com/toolkits/pkg v1.1.3
	github.com/ugorji/go/codec v1.1.7
	github.com/unrolled/render v1.0.3
)

// Fix legacy import path - https://github.com/uber-go/atomic/pull/60
replace github.com/uber-go/atomic => github.com/uber-go/atomic v1.4.0
