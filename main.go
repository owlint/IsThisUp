package main

import (
    "log"
    "os"
    "fmt"
    "encoding/json"
    "strconv"
    "time"
    "bytes"
    "net/http"
    "github.com/sparrc/go-ping"
)

func isConnectedToInternet() bool {
    log.Println("Test internet connection")
    
    pinger, err := ping.NewPinger("www.google.com")
    pinger.SetPrivileged(true)
    if err != nil {
        log.Fatalln(err)
    }
    
    pinger.Count = 3
    pinger.Run() // blocks until finished
    stats := pinger.Statistics() // get send/receive/rtt stats

    return stats.PacketsRecv > 0
}

func testWebsite(websiteUrl string) bool {
    resp, err := http.Get(websiteUrl)
    if err != nil {
        return false
    }
    defer resp.Body.Close()

    return resp.StatusCode < 400
}

func alertOpsGenie(websiteUrl string, apiKey string) {
    message := fmt.Sprintf("%s is not responding", websiteUrl)
    values := map[string]string{"message": message, "priority": "P1"}
    jsonValue, _ := json.Marshal(values)
    
    client := &http.Client{}
    req, _ := http.NewRequest("POST", "https://api.eu.opsgenie.com/v2/alerts", bytes.NewBuffer(jsonValue))
    req.Header.Add("Content-Type", "application/json")
    req.Header.Add("Authorization", fmt.Sprintf("GenieKey %s", apiKey))

    resp, err := client.Do(req)
    defer resp.Body.Close()

    if err != nil || resp.StatusCode > 400 {
        log.Fatalln("Cannot alert with OpsGenie. Quitting...")
    }
}

func main() {
    websiteUrl, haveValue := os.LookupEnv("URL")
    if !haveValue {
        log.Fatalln("No URL env variable. Quitting...")
    }

    apiKey, haveValue := os.LookupEnv("GENIE_KEY")
    if !haveValue {
        log.Fatalln("No GENIE_KEY env variable. Quitting...")
    }

    sleepingTimeString, haveValue := os.LookupEnv("SLEEP")
    if !haveValue {
        log.Fatalln("No SLEEP env variable. Quitting...")
    }
    
    sleepingTime, err := strconv.Atoi(sleepingTimeString)
    if err != nil {
        log.Fatalln("SLEEP is not valid int. Quitting...")
    }
    

    for {
        isConnectedToInternet := isConnectedToInternet()
        if !isConnectedToInternet {
            log.Fatalln("Cannot connect to internet. Quitting...")
        }

        isUp := testWebsite(websiteUrl)

        if isUp {
            log.Printf("%s is up", websiteUrl)
        } else {
            log.Printf("%s is down", websiteUrl)
        }

        if !isUp && isConnectedToInternet {
            alertOpsGenie(websiteUrl, apiKey)
        }
        
        time.Sleep(time.Duration(sleepingTime) * time.Second)
    }

}
