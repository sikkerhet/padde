package main

import (
        "context"
        "flag"
        "fmt"
        "github.com/ClickHouse/clickhouse-go/v2"
        "github.com/nxadm/tail"
        "github.com/tidwall/gjson"
        "log"
        "log/syslog"
        "os"
        "os/signal"
        "strings"
        "syscall"
        "time"
)

var logger *log.Logger
var dbchan = make(chan string, 100)
var linecount int64 = 0
var lastlines int64 = 0
var errorcount int64 = 0
var timeparsecount int64 = 0

func main() {
        // Set up loging to syslog
        logwriter, err := syslog.New(syslog.LOG_NOTICE, "taylor")
        if err == nil {
                log.SetOutput(logwriter)
        }
        // Supress timestamp - included by syslog
        log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

        // Config
        config_file := flag.String("filename", "/var/log/syslog", "logfilename")
        config_skip := flag.String("skip", "TXT,DNSKEY", "List for DNS types to skip")
        flag.Parse()

        log.Printf("Starting taylor..\n")
        // Make channel for loglines
        logchannel := make(chan string, 100)

        filename := *config_file
        skipfields := strings.Split(*config_skip, ",")

        c := make(chan os.Signal)
        signal.Notify(c, syscall.SIGUSR1)
        go func() {
                for {
                        <-c
                        log.Printf("Signal caught, linecount: %v, errorcount: %v, timeparseerror: %v\n", linecount, errorcount, timeparsecount)
                }
        }()

        go func() {
                for {
                        time.Sleep(600 * time.Second)
                        log.Printf("Lines last 10min: %v, errorcount: %v\n", Abs(linecount-lastlines), errorcount)
                        lastlines = linecount
                }
        }()

        log.Printf("Started dbwriter\n")
        // Start db writer
        go dbwriter(dbchan)

        log.Printf("Started reader of logile: %v\n", filename)
        // Start reader loop
        go filereader(filename, logchannel)

        // Wait for new lines on channel
        for {
                select {
                case newline := <-logchannel:
                        linecount++
                        // Parse line async
                        go parseline(newline, skipfields)

                // Print lincount in log if no new lines for 60s
                case <-time.After(time.Second * 60):
                        log.Printf("linecount: %v\n", linecount)
                }
        }

}

func filereader(filename string, outchan chan string) {
        // Get syslogger to pass to tail.Config
        syslogger := getSysLogger()
        for {
                // Open logfile in "tail" mode, send lines to channel
                t, err := tail.TailFile(filename, tail.Config{ReOpen: true, Follow: true, Logger: syslogger})
                if err != nil {
                        log.Printf("Error opening file: %v\n", filename)
                        panic(err)
                }
                // Blocking loop, pass new lines to outchan
                for line := range t.Lines {
                        outchan <- string(line.Text)
                }
                log.Println("taylor t.Lines closed. Trying reopen")
        }
}

// Parse a single JSON line
func parseline(newline string, skipfields []string) {
        // Only parse dns.type: answer lines
        if gjson.Get(newline, "dns.type").String() != "answer" {
                return
        }
        // Get all answers
        dnsarr := gjson.Get(newline, "dns.answers").Array()
        // If len answers >0 iterate results
        if len(dnsarr) > 0 {

                // Get time from JSON
                timestring := gjson.Get(newline, "timestamp").String()
                thetime, err := time.Parse("2006-01-02T15:04:05.99999-0700", timestring)
                if err != nil {
                        timeparsecount++
                        return
                }
                // Convert to int64
                timestamp := int64(thetime.Unix())

                for _, e := range dnsarr {
                        // Get line as string
                        element := e.String()
                        // Get rrtype
                        rrtype := gjson.Get(element, "rrtype").String()
                        // Check if type should be skipped
                        if stringInSlice(rrtype, skipfields) {
                                // Skip if type is to be skipped
                                return
                        }
                        rrname := gjson.Get(element, "rrname").String()
                        rdata := gjson.Get(element, "rdata").String()
                        // Return if any len = 0 or rrname/rdata contains ,
                        if len(rrname) == 0 || len(rrtype) == 0 || len(rdata) == 0 || strings.Contains(rrname, ",") || strings.Contains(rdata, ",") {
                                // Inc. errorcount if error
                                errorcount++
                                return
                        }
                        //DEBUG print fmt.Printf("%q,%q,%q,%v,%v,1\n", rrname, rdata, rrtype, timestamp, timestamp)
                        dboutline := fmt.Sprintf("'%s','%s','%s',%d,%d,1", rrname, rdata, rrtype, timestamp, timestamp)
                        // Send line to dbwriter
                        dbchan <- dboutline
                }
        }
}

// Helper to check if string is in slice
func stringInSlice(str string, list []string) bool {
        for _, v := range list {
                if v == str {
                        return true
                }
        }
        return false
}

// Helper to get *log.Logger
func getSysLogger() *log.Logger {
        logger, err := syslog.NewLogger(syslog.LOG_INFO, (log.LstdFlags &^ (log.Ldate | log.Ltime)))
        if err != nil {
                log.Fatal(err)
        }
        return logger
}

// Simple Abs function
func Abs(x int64) int64 {
        if x < 0 {
                return -x
        }
        return x
}

// Dbwriter, read lines from channel and do async inserts
// 
// Change db params if you use external db
//
func dbwriter(dbline chan string) {
        var (
                ctx       = context.Background()
                conn, err = clickhouse.Open(&clickhouse.Options{
                        Addr: []string{"127.0.0.1:9000"},
                        Auth: clickhouse.Auth{
                                Database: "padde",
                                Username: "default",
                                Password: "",
                        },
                        //Debug:           true,
                        DialTimeout:     time.Second,
                        MaxOpenConns:    10,
                        MaxIdleConns:    5,
                        ConnMaxLifetime: time.Hour,
                })
        )
        // Fail if unable to connect to db
        if err != nil {
                log.Fatal(err)
        }
        for {
                // Wait for new lines, insert into DB
                select {
                // Get newline from dbline channel
                case newline := <-dbline:
                        // Build INSERT string
                        linevalues := "INSERT INTO padde.log VALUES(" + newline + ")"
                        // Async db insert
                        err := conn.AsyncInsert(ctx, linevalues, false)
                        if err != nil {
                                log.Println("Error on DB insert %v", err)
                        }
                }
        }
}
