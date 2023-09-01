package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"slices"
	"strings"
	"time"

	"alexejk.io/go-xmlrpc"
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

type ChallengeResponse struct {
	AuthScheme string `xmlrpc:"auth_scheme"`
	Challenge  string
	ExpireTime string `xmlrpc:"expire_time"`
	ServerTime string `xmlrpc:"server_time"`
}

func (r ChallengeResponse) generateResponse(password string) string {
	ph := fmt.Sprintf("%x", md5.Sum([]byte(password)))
	rh := fmt.Sprintf("%x", md5.Sum([]byte(r.Challenge+ph)))
	return rh
}

func getChallenge(client *xmlrpc.Client) ChallengeResponse {
	challengeResult := &struct {
		Challenge ChallengeResponse
	}{}
	err := client.Call("LJ.XMLRPC.getchallenge", nil, challengeResult)
	if err != nil {
		panic(err)
	}
	return challengeResult.Challenge
}

type LJEntry struct {
	Itemid    int    `xmlrpc:"itemid"`
	Eventtime string `xmlrpc:"eventtime"`
	Security  string `xmlrpc:"security"`
	Subject   string `xmlrpc:"subject"`
	Event     string `xmlrpc:"event"`
	Url       string `xmlrpc:"url"`
	Poster    string `xmlrpc:"poster"`
}

type LJEditRequest struct {
	Username    string `xmlrpc:"username"`
	Method      string `xmlrpc:"auth_method"`
	Challenge   string `xmlrpc:"auth_challenge"`
	Response    string `xmlrpc:"auth_response"`
	Version     int    `xmlrpc:"ver"`
	Itemid      int    `xmlrpc:"itemid"`
	Event       []byte `xmlrpc:"event"`
	Lineendings string `xmlrpc:"lineendings"`
	Subject     string `xmlrpc:"subject"`
	Security    string `xmlrpc:"security"`
}

type LJEditResponse struct {
	Itemid int    `xmlrpc:"itemid"`
	Anum   int    `xmlrpc:"anum"`
	Url    string `xmlrpc:"url"`
}

type LoginRequest struct {
	Username  string `xmlrpc:"username"`
	Method    string `xmlrpc:"auth_method"`
	Challenge string `xmlrpc:"auth_challenge"`
	Response  string `xmlrpc:"auth_response"`
	Version   int    `xmlrpc:"ver"`
}

type LoginResponse struct {
	IsValidated int    `xmlrpc:"is_validated"`
	Userid      int    `xmlrpc:"userid"`
	Username    string `xmlrpc:"username"`
	FullName    string `xmlrpc:"fullname"`
	Message     string
}

type DayCount struct {
	Count int    `xmlrpc:"count"`
	Date  string `xmlrpc:"date"`
}

type GetEventsRequest struct {
	Username   string `xmlrpc:"username"`
	Method     string `xmlrpc:"auth_method"`
	Challenge  string `xmlrpc:"auth_challenge"`
	Response   string `xmlrpc:"auth_response"`
	Version    int    `xmlrpc:"ver"`
	Selecttype string `xmlrpc:"selecttype"`
	Year       string `xmlrpc:"year"`
	Month      string `xmlrpc:"month"`
	Day        string `xmlrpc:"day"`
	Noprops    int    `xmlrpc:"noprops"`
}

func main() {
	var login string
	var password string
	flag.StringVar(&login, "l", "votez", "Login to LiveJournal")
	flag.StringVar(&password, "p", "votez", "User password to LiveJournal")
	flag.Parse()
	fmt.Println("Login to LJ")
	fmt.Println("Login: ", login)

	httpClient := http.Client{
		Transport: &loggingTransport{},
	}

	client, _ := xmlrpc.NewClient("https://www.livejournal.com/interface/xmlrpc", xmlrpc.SkipUnknownFields(true), xmlrpc.HttpClient(&httpClient))
	defer client.Close()

	challengeResult := getChallenge(client)

	type LoginRequestWrapper struct {
		Request LoginRequest
	}

	loginResponse := &struct {
		LoginResponse
	}{}
	err := client.Call("LJ.XMLRPC.login", LoginRequestWrapper{
		Request: LoginRequest{Username: login, Method: "challenge", Version: 1, Challenge: challengeResult.Challenge, Response: challengeResult.generateResponse(password)}}, loginResponse)
	fmt.Println("Got response ", err)
	if err != nil {
		log.Fatal("Got error on Login", err)
		panic(err)
	}
	fmt.Printf("Got userid %d, username %s, full name %s, validated %d", loginResponse.LoginResponse.Userid, loginResponse.LoginResponse.Username, loginResponse.LoginResponse.FullName, loginResponse.LoginResponse.IsValidated)

	dayCountsResponse := &struct {
		Response struct {
			Daycounts []DayCount `xmlrpc:"daycounts"`
		}
	}{}

	challengeResult = getChallenge(client)

	err = client.Call("LJ.XMLRPC.getdaycounts", LoginRequestWrapper{
		Request: LoginRequest{Username: login, Method: "challenge", Version: 1, Challenge: challengeResult.Challenge, Response: challengeResult.generateResponse(password)}}, dayCountsResponse)

	if err != nil {
		log.Fatal("Got error on get day counts", err)
		panic(err)
	}

	type GetEventsRequestWrapper struct {
		Request GetEventsRequest
	}

	type EditEntryRequestWrapper struct {
		Request LJEditRequest
	}

	events := &struct {
		Response struct {
			Events []LJEntry
		}
	}{}

	dayCounts := dayCountsResponse.Response.Daycounts
	slices.Reverse(dayCounts)

	for _, record := range dayCounts {
		fmt.Println("On day ", record.Date, " I had ", record.Count, " records")
		dateArr := strings.Split(record.Date, "-")
		year := dateArr[0]
		month := dateArr[1]
		date := dateArr[2]
		fmt.Println("Year ", year, " month ", month, " date ", date)
		challengeResult = getChallenge(client)
		err = client.Call("LJ.XMLRPC.getevents", GetEventsRequestWrapper{
			Request: GetEventsRequest{
				Username: login, Method: "challenge", Version: 1, Challenge: challengeResult.Challenge, Response: challengeResult.generateResponse(password),
				Selecttype: "day", Year: year, Month: month, Day: date, Noprops: 0,
			}}, events)
		if err != nil {
			fmt.Println("Cannot download entries for ", record.Date)
			log.Fatal(err)
			continue
		}
		for _, entry := range events.Response.Events {
			hasAWS := strings.Contains(entry.Event, "https://s3.eu-central-1.amazonaws.com/")
			hasWrongDomain := strings.Contains(entry.Event, "https://artyukh.hu/")
			if !hasAWS || hasWrongDomain {
				fmt.Println("Cannot find any links in ", entry.Subject)
				continue
			}
			newText := strings.ReplaceAll(entry.Event, "https://s3.eu-central-1.amazonaws.com/", "https://www.artyukh.hu/lj/")
			newText = strings.ReplaceAll(newText, "https://artyukh.hu/", "https://www.artyukh.hu/lj/")
			fmt.Println("Edit entry:", entry.Itemid, entry.Subject)
			editResponse := &struct {
				Response LJEditResponse
			}{}
			challengeResult = getChallenge(client)
			err = client.Call("LJ.XMLRPC.editevent", EditEntryRequestWrapper{
				Request: LJEditRequest{
					Username: login, Method: "challenge", Version: 1, Challenge: challengeResult.Challenge, Response: challengeResult.generateResponse(password),
					Lineendings: "unix", Itemid: entry.Itemid, Event: []byte(newText), Subject: entry.Subject, Security: entry.Security,
				}}, editResponse)
			if err != nil {
				fmt.Println("Cannot update entry", entry.Itemid)
				log.Fatal(err)
				continue
			}
			fmt.Println("Response:", editResponse)
			time.Sleep(5 * time.Second)
		}
		time.Sleep(5 * time.Second)
		// fmt.Println(events)
		// break
	}
}
