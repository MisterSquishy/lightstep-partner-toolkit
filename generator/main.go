package main

import (
	"encoding/json"
	"flag"
	scheduledMetric "github.com/smithclay/synthetic-load-generator-go/generator/metric"
	"github.com/smithclay/synthetic-load-generator-go/generator/trace"
	"github.com/smithclay/synthetic-load-generator-go/topology"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	var paramsFile string
	var collectorUrl string
	var randSeed int64
	stdoutMode := false

	flag.StringVar(&paramsFile, "paramsFile", "REQUIRED", "topology JSON file")
	flag.StringVar(&collectorUrl, "collectorUrl", "", "URL to gRPC OpenTelemetry collector")
	flag.Int64Var(&randSeed, "randSeed", time.Now().UTC().UnixNano(), "random seed (int64)")

	flag.Parse()
	if collectorUrl == "" {
		stdoutMode = true
	}

	jsonFile, err := os.Open(paramsFile)
	if err != nil {
		log.Fatalf("could not open topology file: %v", err)
	}
	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)

	var file topology.File
	err = json.Unmarshal(byteValue, &file)
	if err != nil {
		log.Fatalf("could not parse topology file: %v", err)
	}
	metricGenerators := make([]*scheduledMetric.ScheduledMetricGenerator, 0)
	for _, s := range file.Topology.Services {
		if len(s.Metrics) == 0 {
			continue
		}

		var mg *scheduledMetric.ScheduledMetricGenerator
		mg = scheduledMetric.NewScheduledMetricGenerator(s.Metrics, s.ServiceName,
			scheduledMetric.WithSeed(randSeed),
			scheduledMetric.WithMetricsPerHour(3600),
			scheduledMetric.WithGrpc(collectorUrl),
		)

		if stdoutMode {
			mg = scheduledMetric.NewScheduledMetricGenerator(s.Metrics, s.ServiceName,
				scheduledMetric.WithSeed(randSeed),
				scheduledMetric.WithMetricsPerHour(3600),
			)
		}
		metricGenerators = append(metricGenerators, mg)
	}
	traceGenerators := make([]*trace.ScheduledTraceGenerator, 0)
	for _, r := range file.RootRoutes {
		var tg *trace.ScheduledTraceGenerator

		tg = trace.NewScheduledTraceGenerator(file.Topology, r.Route, r.Service,
			trace.WithSeed(randSeed),
			trace.WithTracesPerHour(r.TracesPerHour),
			trace.WithGrpc(collectorUrl))

		if stdoutMode {
			tg = trace.NewScheduledTraceGenerator(file.Topology, r.Route, r.Service,
				trace.WithSeed(randSeed),
				trace.WithTracesPerHour(r.TracesPerHour))
		}

		traceGenerators = append(traceGenerators, tg)
	}

	for _, tg := range traceGenerators {
		go tg.Start()
	}

	for _, mg := range metricGenerators {
		go mg.Start()
	}

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Println("Shutting down...")
	for _, tg := range traceGenerators {
		tg.Shutdown()
	}
	for _, mg := range metricGenerators {
		mg.Shutdown()
	}
	os.Exit(0)

}
