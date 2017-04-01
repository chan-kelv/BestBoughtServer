package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

var (
	MICROSOFT_COG_KEY = "9122668a88454ac9bed0b816edbe5c8c"
)

func main() {
	fmt.Println("Start server")
	server := mux.NewRouter().StrictSlash(true)
	server.Headers("Content-Type", "application/json")
	server.HandleFunc("/help", HelpRoute).Methods("GET")
	server.HandleFunc("/", Index).Methods("GET")
	server.HandleFunc("/hello/{name}", Hello)
	server.HandleFunc("/product/{prodId}", CommentNLP).Methods("GET")
	server.HandleFunc("/dev/", Dev)
	// http.Handle("/", server)
	log.Fatal(http.ListenAndServe(":8080", server))
}

//================Routes============

func Index(w http.ResponseWriter, req *http.Request) {
	fmt.Println("Hello there")
	fmt.Fprintf(w, "Hello there!")
}

func Hello(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	name := vars["name"]
	fmt.Println("Name:", name)
	fmt.Fprintf(w, "Hello %s", name)
}

func HelpRoute(w http.ResponseWriter, req *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")
	resp, err := httpGet("http://www.bestbuy.ca/api/v2/json/reviews/10415309?page=1&pagesize=50&source=us")
	if err != nil {
		fmt.Println("GET error:", err)
		return
	}
	//all good in the hood
	if resp.StatusCode >= 200 && resp.StatusCode <= 300 {
		//get the json response body as []byte
		respBodyBytes, _ := ioutil.ReadAll(resp.Body)
		fmt.Fprintf(w, string(respBodyBytes))
		return
	}
}

func CommentNLP(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	prodIdRaw := vars["prodId"]
	// prodId, err := strconv.Atoi(prodIdRaw)
	// if err != nil {
	// 	fmt.Println("Product num", prodIdRaw, "error:", err)
	// 	fmt.Fprintf(w, "Could not read product", prodIdRaw)
	// 	return
	// }

	resp, err := httpGet("http://www.bestbuy.ca/api/v2/json/reviews/" + prodIdRaw + "?page=1&pagesize=50&source=us")
	if err != nil {
		fmt.Println("GET error:", err)
		return
	}
	//all good in the hood
	if resp.StatusCode >= 200 && resp.StatusCode <= 300 {
		//get the json response body as []byte
		respBodyBytes, _ := ioutil.ReadAll(resp.Body)

		//get the comments as []string
		commStruct := getCommentsFromResp(respBodyBytes)

		//json package for microsoft cog labs
		microsoftJson, nlpComment := parseCommentsForMicrosoft(commStruct)

		//call sentiment analysis
		microsoftSentiment(microsoftJson, nlpComment)
		microsoftKeyWords(microsoftJson, nlpComment)

		//nlp comment is now ready to parse for key words
		batteryWordCount(nlpComment.CommStruct)
		//TODO add more attributes

		prodRanked(nlpComment) //prod will have rankscores

		rankComments(nlpComment)

		final, err := json.Marshal(nlpComment)
		if err != nil {
			fmt.Println("error:", err)
		}
		fmt.Fprintf(w, string(final))
		fmt.Println(nlpComment)
	}
}

func Dev(w http.ResponseWriter, req *http.Request) {
}

//=============Helpers==================
func getCommentsFromResp(respBodyBytes []byte) []CommStruct {
	var dat map[string]interface{}
	if err := json.Unmarshal(respBodyBytes, &dat); err != nil {
		panic(err)
	}

	var commStruct []CommStruct
	for _, val := range dat["reviews"].([]interface{}) {
		v := val.(map[string]interface{})
		for k2, v2 := range v {
			var c CommStruct
			if k2 == "comment" {
				c.Comment = v2.(string)
			}
			if k2 == "rating" {
				c.Rating = v2.(float64)
			}
			commStruct = append(commStruct, c)
		}
	}
	return commStruct
}

func parseCommentsForMicrosoft(comments []CommStruct) ([]byte, NlpComment) {
	//TODO: stop if ID>100
	idCount := 1
	// commentMap := make(map[string]NlpComment)
	// commentMap[strconv.Itoa(idCount)] = NlpComment{commStruct: comment}
	var d Document
	for _, comment := range comments {
		commElement := DocElement{Language: "en", Id: strconv.Itoa(idCount), Text: comment.Comment}
		d.Documents = append(d.Documents, commElement)
		idCount++
	}
	j, err := json.Marshal(d)
	if err != nil {
		fmt.Println("json marshal error", err)
		panic(err)
	}
	return j, NlpComment{CommStruct: comments}
}

func microsoftSentiment(jsonByte []byte, nlpComm NlpComment) {
	url := "https://westus.api.cognitive.microsoft.com/text/analytics/v2.0/sentiment"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonByte))
	req.Header.Set("Ocp-Apim-Subscription-Key", MICROSOFT_COG_KEY)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body) //[]byte of response

	var sentResp SentimentResponse
	err = json.Unmarshal(body, &sentResp)
	if err != nil {
		fmt.Println("JSON unmarshall error:", err)
		return
	}
	i := 0
	for _, sent := range sentResp.Documents {
		// s := nlpComm[sent.Id]
		// s.sentimentScore = sent.Score
		// nlpComm[sent.Id] = s
		nlpComm.CommStruct[i].SentimentScore = sent.Score
		i++
	}
}

func microsoftKeyWords(jsonByte []byte, nlpComm NlpComment) {
	url := "https://westus.api.cognitive.microsoft.com/text/analytics/v2.0/keyPhrases"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonByte))
	req.Header.Set("Ocp-Apim-Subscription-Key", MICROSOFT_COG_KEY)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body) //resp body as []byte
	// var x map[string]interface{}
	// json.Unmarshal(byte[](body), &x)
	// fmt.Println("\n\n")
	var keyResp KeywordResponse
	err = json.Unmarshal(body, &keyResp)
	if err != nil {
		fmt.Println("JSON unmarshall error:", err)
		return
	}

	var i int = 0
	for _, phrases := range keyResp.Documents {
		// k := nlpComm[phrases.Id]
		// k.keyPhrases = phrases.KeyPhrases
		// nlpComm[phrases.Id] = k
		nlpComm.CommStruct[i].KeyPhrase = phrases.KeyPhrases
		i++
	}
}

func httpGet(url string) (*http.Response, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET error", err)
	}
	return resp, nil
}

func batteryWordCount(comm []CommStruct) {
	for _, comm := range comm {
		goodBattery := 0
		badBattery := 0
		for _, phrase := range comm.KeyPhrase {
			if strings.Contains(strings.ToLower(phrase), "battery") {
				fmt.Println("Battery found!", phrase)
				if comm.SentimentScore > 0.5 {
					goodBattery++
				} else {
					badBattery++
				}
			}
		}
		comm.GoodBattery = goodBattery
		comm.BadBattery = badBattery
	}
}

func prodRanked(nComm NlpComment) {
	prodRankMap := make(map[float64]CommStruct) //rankScore:comment HACK - assume unique
	var scoreList []float64
	for _, comm := range nComm.CommStruct {
		var rankScore float64 = 0
		if containProCon(comm.KeyPhrase) {
			rankScore = rankScore + 10
		}

		if comm.GoodBattery != 0 {
			rankScore = rankScore + 1
		} else if comm.BadBattery != 0 {
			rankScore = rankScore + 1
		}

		rankScore = rankScore + (comm.SentimentScore * 10)

		comm.NlpRank = rankScore
		prodRankMap[rankScore] = comm
		scoreList = append(scoreList, rankScore)
	}

	sort.Float64s(scoreList)
	var rankedList []CommStruct
	for i := len(scoreList) - 1; i >= 0; i-- {
		rankedList = append(rankedList, prodRankMap[scoreList[i]])
	}
	nComm.CommStruct = rankedList
}

func rankComments(nComm NlpComment) {
	var goodS []string
	var badS []string
	for _, c := range nComm.CommStruct {
		if c.Rating >= 4 {
			goodS = append(goodS, c.Comment)
		} else {
			badS = append(badS, c.Comment)
		}
	}
	nComm.GoodComments = goodS
	nComm.BadComments = badS
}

func containProCon(phrases []string) bool {
	for _, p := range phrases {
		if strings.Contains(p, "pro") || strings.Contains(p, "pros") ||
			strings.Contains(p, "con") || strings.Contains(p, "cons") {
			return true
		}
	}
	return false
}

//=============Structs================
type DocElement struct {
	Language string `json:"language"`
	Id       string `json:"id"`
	Text     string `json:"text"`
}

type Document struct {
	Documents []DocElement `json:"documents"`
}

type Sentiment struct {
	Score float64 `json:"score"`
	Id    string  `json:"id"`
}

type SentimentResponse struct {
	Documents []Sentiment `json:"documents"`
	Errors    []string    `json:"errors"`
}

type KeywordResponse struct {
	Documents []KeyPhrase `json:"documents"`
	Errors    []string    `json:"errors"`
}

type KeyPhrase struct {
	KeyPhrases []string `json:"keyPhrases"`
	Id         string   `json:"id"`
}

type CommStruct struct {
	Comment        string   `json:"CommentText"`
	Rating         float64  `json:"rating"`
	KeyPhrase      []string `json:"keyPhrase"`
	NlpRank        float64  `json:"nlpRank"`
	SentimentScore float64  `json:"sentimentScore"`
	GoodBattery    int      `json:"goodB"`
	BadBattery     int      `json:"badB"`
}

type NlpComment struct {
	CommStruct   []CommStruct `json:"comments"`
	GoodComments []string     `json:"goodComments"`
	BadComments  []string     `json:"badComments"`
}
