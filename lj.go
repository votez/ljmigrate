package main

import (
	"crypto/md5"
	"fmt"
	"log"
	"strings"

	"alexejk.io/go-xmlrpc"
	"go.uber.org/ratelimit"
)

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

func (lj *LJ) getChallenge() ChallengeResponse {
	challengeResult := &struct {
		Challenge ChallengeResponse
	}{}
	lj.limiter.Take()
	err := lj.xmlrpcClient.Call("LJ.XMLRPC.getchallenge", nil, challengeResult)
	if err != nil {
		log.Fatal("Cannot get challenge", err)
		panic(err)
	}
	return challengeResult.Challenge
}

type LJ struct {
	xmlrpcClient *xmlrpc.Client
	username     string
	password     string
	limiter      ratelimit.Limiter
}

const LjUrl = "https://www.livejournal.com/interface/xmlrpc"
const botRespectLimit = 4 // per second

func NewLJwithXmlRpcClient(client *xmlrpc.Client, user string, password string) LJ {
	return LJ{
		xmlrpcClient: client,
		username:     user,
		password:     password,
		limiter:      ratelimit.New(botRespectLimit),
	}
}

func NewLJ(user string, password string) (*LJ, error) {
	client, err := xmlrpc.NewClient("https://www.livejournal.com/interface/xmlrpc", xmlrpc.SkipUnknownFields(true))
	if err != nil {
		log.Fatal("Cannot create the client", err)
		return nil, err
	}
	return &LJ{
		xmlrpcClient: client,
		username:     user,
		password:     password,
		limiter:      ratelimit.New(botRespectLimit),
	}, nil
}

func (lj *LJ) Close() {
	lj.xmlrpcClient.Close()
}

func (lj *LJ) Login() (*LoginResponse, error) {
	type LoginRequestWrapper struct {
		Request LoginRequest
	}

	loginResponse := &struct {
		LoginResponse
	}{}

	challengeResult := lj.getChallenge()
	lj.limiter.Take()
	err := lj.xmlrpcClient.Call("LJ.XMLRPC.login", LoginRequestWrapper{
		Request: LoginRequest{Username: lj.username, Method: "challenge", Version: 1, Challenge: challengeResult.Challenge, Response: challengeResult.generateResponse(lj.password)}}, loginResponse)
	if err != nil {
		log.Fatal("Got error on Login", err)
		return nil, err
	}
	return &loginResponse.LoginResponse, nil
}

func (lj *LJ) GetDayCounts() (*[]DayCount, error) {
	type LoginRequestWrapper struct {
		Request LoginRequest
	}
	dayCountsResponse := &struct {
		Response struct {
			Daycounts []DayCount `xmlrpc:"daycounts"`
		}
	}{}
	challengeResult := lj.getChallenge()
	lj.limiter.Take()
	err := lj.xmlrpcClient.Call("LJ.XMLRPC.getdaycounts", LoginRequestWrapper{
		Request: LoginRequest{Username: lj.username, Method: "challenge", Version: 1, Challenge: challengeResult.Challenge, Response: challengeResult.generateResponse(lj.password)}}, dayCountsResponse)

	if err != nil {
		log.Fatal("Cannot get days count", err)
		return nil, err
	}
	return &dayCountsResponse.Response.Daycounts, nil

}

func (lj *LJ) GetEvents(date string) (*[]LJEntry, error) {
	type GetEventsRequestWrapper struct {
		Request GetEventsRequest
	}

	events := &struct {
		Response struct {
			Events []LJEntry
		}
	}{}

	dateArr := strings.Split(date, "-")
	year := dateArr[0]
	month := dateArr[1]
	day := dateArr[2]
	challengeResult := lj.getChallenge()
	lj.limiter.Take()
	err := lj.xmlrpcClient.Call("LJ.XMLRPC.getevents", GetEventsRequestWrapper{
		Request: GetEventsRequest{
			Username: lj.username, Method: "challenge", Version: 1, Challenge: challengeResult.Challenge, Response: challengeResult.generateResponse(lj.password),
			Selecttype: "day", Year: year, Month: month, Day: day, Noprops: 0,
		}}, events)
	if err != nil {
		log.Fatal("Cannot download entries for ", date, err)
		return nil, err
	}
	return &events.Response.Events, nil
}

func (lj *LJ) EditEntry(itemid int, newText string, newSubject string, newSecurity string) (*LJEditResponse, error) {
	type EditEntryRequestWrapper struct {
		Request LJEditRequest
	}
	editResponse := &struct {
		Response LJEditResponse
	}{}
	challengeResult := lj.getChallenge()
	lj.limiter.Take()
	err := lj.xmlrpcClient.Call("LJ.XMLRPC.editevent", EditEntryRequestWrapper{
		Request: LJEditRequest{
			Username: lj.username, Method: "challenge", Version: 1, Challenge: challengeResult.Challenge, Response: challengeResult.generateResponse(lj.password),
			Lineendings: "unix", Itemid: itemid, Event: []byte(newText), Subject: newSubject, Security: newSecurity,
		}}, editResponse)
	if err != nil {
		log.Fatal("Cannot edit entry", itemid, err)
		return nil, err
	}
	return &editResponse.Response, nil
}
