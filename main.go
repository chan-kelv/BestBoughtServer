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
	resp, err := httpGet("http://www.bestbuy.ca/api/v2/json/reviews/10415309?page=1&pagesize=20&source=us")
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

	resp, err := httpGet("http://www.bestbuy.ca/api/v2/json/reviews/" + prodIdRaw + "?page=1&pagesize=20&source=us")
	if err != nil {
		fmt.Println("GET error:", err)
		return
	}
	//all good in the hood
	if resp.StatusCode >= 200 && resp.StatusCode <= 300 {
		//get the json response body as []byte
		respBodyBytes, _ := ioutil.ReadAll(resp.Body)

		//get the comments as []string
		commStruct := getCommentsFromResp(respBodyBytes) //commStruct

		//json package for microsoft cog labs
		microsoftJson, nlpCommentMap := parseCommentsForMicrosoft(commStruct)

		//call sentiment analysis
		microsoftSentiment(microsoftJson, nlpCommentMap)
		microsoftKeyWords(microsoftJson, nlpCommentMap)

		//nlp comment is now ready to parse for key words
		batteryWordCount(nlpCommentMap)
		//TODO add more attributes

		ranked := prodRanked(nlpCommentMap) //prod will have rankscores

		rankComments(ranked)

		final, err := json.Marshal(ranked)
		if err != nil {
			fmt.Println("error:", err)
		}
		fmt.Fprintf(w, string(final))
		// fmt.Println(ranked)
	}
}

func Dev(w http.ResponseWriter, req *http.Request) {
	// resp, err := httpGet("http://www.bestbuy.ca/api/v2/json/reviews/10415309?page=1&pagesize=20&source=us")
	// if err != nil {
	// 	fmt.Println("GET error:", err)
	// 	return
	// }
	// //all good in the hood
	// if resp.StatusCode >= 200 && resp.StatusCode <= 300 {
	// 	//get the json response body as []byte
	// 	respBodyBytes, _ := ioutil.ReadAll(resp.Body)
	//
	// 	//get the comments as []string
	// 	comments := getCommentsFromResp(respBodyBytes)
	//
	// 	//json package for microsoft cog labs
	// 	microsoftJson, nlpCommentMap := parseCommentsForMicrosoft(comments)
	//
	// 	//call sentiment analysis
	// 	microsoftSentiment(microsoftJson, nlpCommentMap)
	// 	microsoftKeyWords(microsoftJson, nlpCommentMap)
	//
	// 	//nlp comment is now ready to parse for key words
	// 	batteryWordCount(nlpCommentMap)
	//
	// 	fmt.Println(nlpCommentMap)
	// }
}

//=============Helpers==================
type CommStruct struct {
	Comments []string
	Rating   float64
}

func getCommentsFromResp(respBodyBytes []byte) CommStruct {
	var dat map[string]interface{}
	if err := json.Unmarshal(respBodyBytes, &dat); err != nil {
		panic(err)
	}

	var commStruct CommStruct
	for _, val := range dat["reviews"].([]interface{}) {
		v := val.(map[string]interface{})
		for k2, v2 := range v {
			if k2 == "comment" {
				commStruct.Comments = append(commStruct.Comments, v2.(string))
			}
			if k2 == "rating" {
				commStruct.Rating = v2.(float64)
			}
		}
	}
	return commStruct
}

func parseCommentsForMicrosoft(comments CommStruct) ([]byte, map[string]NlpComment) {
	//TODO: stop if ID>100
	idCount := 1
	commentMap := make(map[string]NlpComment)
	var d Document
	for _, comment := range comments.Comments {
		commElement := DocElement{Language: "en", Id: strconv.Itoa(idCount), Text: comment}
		d.Documents = append(d.Documents, commElement)

		commentMap[strconv.Itoa(idCount)] = NlpComment{CommentText: comment, ReviewScore: comments.Rating}

		idCount++
	}
	j, err := json.Marshal(d)
	if err != nil {
		fmt.Println("json marshal error", err)
		return nil, nil
	}
	return j, commentMap
}

func microsoftSentiment(jsonByte []byte, nlpComm map[string]NlpComment) {
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

	for _, sent := range sentResp.Documents {
		s := nlpComm[sent.Id]
		s.SentimentScore = sent.Score
		nlpComm[sent.Id] = s
	}
}

func microsoftKeyWords(jsonByte []byte, nlpComm map[string]NlpComment) {
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
	for _, phrases := range keyResp.Documents {
		// s := nlpComm[sent.Id]
		// s.SentimentScore = sent.Score
		// nlpComm[sent.Id] = s
		k := nlpComm[phrases.Id]
		k.KeyPhrases = phrases.KeyPhrases
		nlpComm[phrases.Id] = k
	}
}

func httpGet(url string) (*http.Response, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET error", err)
	}
	return resp, nil
}

func batteryWordCount(nlpComm map[string]NlpComment) {

	for id, comm := range nlpComm {
		goodBattery := 0
		badBattery := 0
		for _, phrase := range comm.KeyPhrases {
			if strings.Contains(strings.ToLower(phrase), "battery") {
				fmt.Println("Battery found!", phrase)
				if comm.SentimentScore > 0.5 {
					goodBattery++
				} else {
					badBattery++
				}
			}
		}
		temp := nlpComm[id]
		temp.GoodBattery = goodBattery
		temp.BadBattery = badBattery
		nlpComm[id] = temp
	}
}

func prodRanked(prodMap map[string]NlpComment) []NlpComment {
	prodRankMap := make(map[float64]NlpComment) //rankScore:comment HACK - assume unique
	var scoreList []float64
	for _, comm := range prodMap {
		var rankScore float64 = 0
		if containProCon(comm.KeyPhrases) {
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
	var rankedList []NlpComment
	for i := len(scoreList) - 1; i >= 0; i-- {
		rankedList = append(rankedList, prodRankMap[scoreList[i]])
	}
	return rankedList
}

func rankComments(comments []NlpComment) {
	for _, c := range comments {
		if c.ReviewScore >= 4 {
			c.GoodComments = append(c.GoodComments, c.CommentText)
		} else {
			c.BadComments = append(c.BadComments, c.CommentText)
		}
	}
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

type NlpComment struct {
	CommentText    string
	GoodComments   []string
	BadComments    []string
	SentimentScore float64
	KeyPhrases     []string
	GoodBattery    int
	BadBattery     int
	NlpRank        float64
	ReviewScore    float64
}
