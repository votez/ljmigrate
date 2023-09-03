package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"slices"
	"strings"
)

type loggingTransport struct{}

func (s *loggingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	bytes, _ := httputil.DumpRequestOut(r, true)

	resp, err := http.DefaultTransport.RoundTrip(r)
	// err is returned after dumping the response

	respBytes, _ := httputil.DumpResponse(resp, true)
	bytes = append(bytes, respBytes...)

	fmt.Printf("%s\n", bytes)

	return resp, err
}

func main() {
	var login string
	var password string
	flag.StringVar(&login, "l", "votez", "Login to LiveJournal")
	flag.StringVar(&password, "p", "votez", "User password to LiveJournal")
	flag.Parse()

	client, _ := NewLJ(login, password)
	defer client.Close()

	loginResponse, err := client.Login()
	fmt.Println("Got response ", err)
	if err != nil {
		log.Fatal("Got error on Login", err)
		panic(err)
	}
	fmt.Printf("Got userid %d, username %s, full name %s, validated %d", loginResponse.Userid, loginResponse.Username, loginResponse.FullName, loginResponse.IsValidated)

	dayCountsResponse, err := client.GetDayCounts()
	if err != nil {
		log.Fatal("Got error on get day counts", err)
		panic(err)
	}

	slices.Reverse(*dayCountsResponse)

	for _, record := range *dayCountsResponse {
		fmt.Println("On day ", record.Date, " I had ", record.Count, " records")
		events, err := client.GetEvents(record.Date)
		if err != nil {
			fmt.Println("Cannot download entries for ", record.Date)
			log.Fatal(err)
			continue
		}
		for _, entry := range *events {
			hasAWS := strings.Contains(entry.Event, "https://s3.eu-central-1.amazonaws.com/")
			hasWrongDomain := strings.Contains(entry.Event, "https://artyukh.hu/")
			if !hasAWS || hasWrongDomain {
				fmt.Println("Cannot find any links in ", entry.Subject, " on ", record.Date, " with ID ", entry.Itemid, " with URL ", entry.Url)
				continue
			}
			fmt.Println("Will edit ", entry.Subject, " on ", record.Date, " with ID ", entry.Itemid, " with URL ", entry.Url)
			newText := strings.ReplaceAll(entry.Event, "https://s3.eu-central-1.amazonaws.com/", "https://www.artyukh.hu/lj/")
			editResponse, err := client.EditEntry(entry.Itemid, newText, entry.Subject, entry.Security)
			if err != nil {
				fmt.Println("Cannot update entry", entry.Itemid)
				log.Fatal(err)
				continue
			}
			fmt.Println("Response:", editResponse)
		}
	}
}
