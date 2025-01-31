package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vaibhav-sinha/jobqueue"
	"github.com/vaibhav-sinha/jobqueue/mongodb"
	"github.com/vaibhav-sinha/jobqueue/mysql"
)

var (
	errProcessorFailed = errors.New("processor failed")
)

func main() {
	const (
		exampleDBURL = "root@tcp(127.0.0.1:3306)/jobqueue_e2e?loc=UTC&parseTime=true"
	)
	var (
		ranks           = flag.Int("r", 1, "number of ranks as in [0,r)")
		concurrency     = flag.Int("c", 2, "maximum number of workers")
		fillTime        = flag.Duration("fill-time", 5*time.Second, "interval in which new jobs get added")
		runTime         = flag.Duration("run-time", 7*time.Second, "maximum run time of a single job")
		logInterval     = flag.Duration("log-interval", 1*time.Second, "log interval for stats")
		maxRetry        = flag.Int("max-retry", 2, "maximum number of retries per job")
		dbtype          = flag.String("dbtype", "mysql", "Storage type (memory, mysql or mongodb)")
		dburl           = flag.String("dburl", "", "MySQL or MongoDB connection string for persistent storage, e.g. "+exampleDBURL)
		dbdebug         = flag.Bool("dbdebug", false, "Enabled debug output for DB store")
		topicsList      = flag.String("topics", "a,b,c", "comma-separated list of topics")
		failureRate     = flag.Float64("failure-rate", 0.05, "failure rate in the interval [0.0,1.0]")
		shutdownTimeout = flag.Duration("shutdown-timeout", -1*time.Second, "timeout to wait after shutdown (negative to wait forever)")
	)
	flag.Parse()

	if *ranks <= 0 {
		log.Fatal("r must be greater than 0")
	}
	if *dburl == "" {
		log.Fatal("specify a database connection string with -dburl like e.g. " + exampleDBURL)
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	rand.Seed(time.Now().UnixNano())

	// Initialize the store
	var err error
	var store jobqueue.Store
	switch *dbtype {
	case "mysql":
		var dboptions []mysql.StoreOption
		if *dbdebug {
			dboptions = append(dboptions, mysql.SetDebug(true))
		}
		store, err = mysql.NewStore(*dburl, dboptions...)
	case "mongodb":
		var dboptions []mongodb.StoreOption
		store, err = mongodb.NewStore(*dburl, dboptions...)
	case "memory":
	default:
		log.Fatal("unsupported dbtype; use either mysql or mongodb")
	}
	if err != nil {
		log.Fatal(err)
	}

	// Initialize the manager
	var options []jobqueue.ManagerOption
	if store != nil {
		options = append(options, jobqueue.SetStore(store))
	}
	for rank := 0; rank < *ranks; rank++ {
		options = append(options, jobqueue.SetConcurrency(rank, *concurrency))
	}
	m := jobqueue.New(options...)

	// Add topics and processors
	topics := strings.SplitN(*topicsList, ",", -1)
	for _, topic := range topics {
		err := m.Register(topic, makeProcessor(topic, *failureRate, *runTime))
		if err != nil {
			log.Fatal(err)
		}
	}

	// Start the manager
	err = m.Start()
	if err != nil {
		log.Fatal(err)
	}

	errc := make(chan error, 1)

	// Enqueue tasks
	go func() {
		errc <- enqueuer(m, topics, *ranks, *fillTime, *maxRetry)
	}()

	// Print stats
	go logger(m, *logInterval)

	// Wait for e.g. Ctrl+C
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
		log.Printf("signal %v", fmt.Sprint(<-c))
		errc <- m.CloseWithTimeout(*shutdownTimeout)
	}()

	if err := <-errc; err != nil {
		log.Fatal(err)
	} else {
		log.Print("exiting")
	}
}

func enqueuer(m *jobqueue.Manager, topics []string, ranks int, fillTime time.Duration, maxRetry int) error {
	var cnt int

	fillTimeNanos := fillTime.Nanoseconds()
	for {
		time.Sleep(time.Duration(rand.Int63n(fillTimeNanos)) * time.Nanosecond)
		topic := topics[rand.Intn(len(topics))]
		rank := rand.Intn(ranks)
		cnt++
		cid := fmt.Sprintf("#%05d", cnt)
		job := &jobqueue.Job{Topic: topic, Rank: rank, MaxRetry: maxRetry, CorrelationID: cid}
		err := m.Add(context.Background(), job)
		if err != nil {
			return err
		}
	}
}

func logger(m *jobqueue.Manager, d time.Duration) {
	for range time.Tick(d) {
		ss, err := m.Stats(context.Background(), &jobqueue.StatsRequest{})
		if err == nil {
			fmt.Printf("Waiting=%6d Working=%6d Succeeded=%6d Failed=%6d\n",
				ss.Waiting,
				ss.Working,
				ss.Succeeded,
				ss.Failed)
		}
	}
}

func makeProcessor(topic string, failureRate float64, runTime time.Duration) jobqueue.Processor {
	runTimeNanos := runTime.Nanoseconds()
	return func(job *jobqueue.Job) error {
		time.Sleep(time.Duration(rand.Int63n(runTimeNanos)) * time.Nanosecond)
		if rand.Float64() < failureRate {
			return errProcessorFailed
		}
		return nil
	}
}
