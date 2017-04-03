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
	MICROSOFT_COG_KEY          = "9122668a88454ac9bed0b816edbe5c8c"
	SEMANTIC_THRESHOLD         = 0.5 //determins if a attribute is pos/neg
	KEY_ATTR_WEIGHT    float64 = 10
	MINOR_ATTR_WEIGHT  float64 = 1
)

func main() {
	fmt.Println("Start server")
	server := mux.NewRouter().StrictSlash(true)
	server.Headers("Content-Type", "application/json")
	server.HandleFunc("/help", HelpRoute).Methods("GET")
	server.HandleFunc("/", Index).Methods("GET")
	// server.HandleFunc("/hello/{name}", Hello)
	server.HandleFunc("/product/{prodId}", CommentNLP).Methods("GET")
	// server.HandleFunc("/dev/", Dev)
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
	//get the product id from the url
	vars := mux.Vars(req)
	prodIdRaw := vars["prodId"]

	//best buy api for product info
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
		commStructs := getCommentsFromResp(respBodyBytes) //commStruct

		// //json package for microsoft cog labs
		microsoftJson := parseCommentsForMicrosoft(commStructs)
		//
		// //call sentiment analysis
		microsoftSentiment(microsoftJson, commStructs)
		//call keyword analysis
		microsoftKeyWords(microsoftJson, commStructs)
		//
		// nlp comment is now ready to parse for key words
		batteryWordCount(commStructs)
		// //TODO add more attributes
		//
		prodRanked(commStructs) //prod will have rankscores

		//rank the comm structs with the new nlp score
		sort.Slice(commStructs, func(i int, j int) bool {
			return commStructs[i].NlpRankScore > commStructs[j].NlpRankScore //from highest to lowers
		})

		nlpProduct := NlpProduct{Comments: commStructs}
		goodComm, badComm := sortComments(nlpProduct)
		nlpProduct.GoodComments = goodComm
		nlpProduct.BadComments = badComm

		final, err := json.Marshal(nlpProduct)
		if err != nil {
			fmt.Println("final marshall error:", err)
		}
		fmt.Fprintf(w, string(final))
		fmt.Println("Product retrieved:", prodIdRaw)
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
func httpGet(url string) (*http.Response, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET error", err)
	}
	return resp, nil
}

//From best buy comment response, return a list of comments with features we want in []NlpComment
func getCommentsFromResp(respBodyBytes []byte) []NlpComment {
	var bbData BestBuyProd
	if err := json.Unmarshal(respBodyBytes, &bbData); err != nil {
		panic(err)
	}

	var nlpComms []NlpComment
	for _, comment := range bbData.Reviews {
		nlpComm := NlpComment{CustomerComment: comment.Comment, CustomerRating: comment.Rating}
		nlpComms = append(nlpComms, nlpComm)
	}

	return nlpComms
}

//for microsoft semantics, they need a []byte for each comment
func parseCommentsForMicrosoft(comments []NlpComment) []byte {
	//TODO: stop if ID>100
	idCount := 1
	// commentMap := make(map[string]NlpComment)
	//put all comments into document to be processed as json arr
	var d Document
	for _, comm := range comments {
		commElement := DocElement{Language: "en", Id: strconv.Itoa(idCount), Text: comm.CustomerComment}
		d.Documents = append(d.Documents, commElement)

		// commentMap[strconv.Itoa(idCount)] = NlpComment{CommentText: comment, ReviewScore: comments.Rating}
		idCount++
	}

	j, err := json.Marshal(d)
	if err != nil {
		fmt.Println("json marshal error", err)
		return nil
	}
	return j
}

func microsoftSentiment(jsonByte []byte, nlpComm []NlpComment) {
	url := "https://westus.api.cognitive.microsoft.com/text/analytics/v2.0/sentiment"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonByte))
	//required headers
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

	//for each sentimate store, add it to the comments
	for i := 0; i < len(nlpComm); i++ {
		nlpComm[i].SentimentScore = sentResp.Documents[i].Score
	}
}

func microsoftKeyWords(jsonByte []byte, nlpComm []NlpComment) {
	//TODO stop at id > 100
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

	for i := 0; i < len(nlpComm); i++ {
		nlpComm[i].KeyPhrases = keyResp.Documents[i].KeyPhrases
	}
	// for _, phrases := range keyResp.Documents {
	// 	// s := nlpComm[sent.Id]
	// 	// s.SentimentScore = sent.Score
	// 	// nlpComm[sent.Id] = s
	// 	k := nlpComm[phrases.Id]
	// 	k.KeyPhrases = phrases.KeyPhrases
	// 	nlpComm[phrases.Id] = k
	// }
}

func batteryWordCount(nlpComms []NlpComment) {
	//for each comment
	for _, comm := range nlpComms {
		goodBattery := 0
		badBattery := 0
		//check a comments phrases for the word battery
		for _, phrase := range comm.KeyPhrases {
			//TODO check if regex is needed
			if (strings.Contains(strings.ToLower(phrase), "battery")) ||
				(strings.Contains(strings.ToLower(phrase), "batteries")) {
				//if battery found compare to semantics
				if comm.SentimentScore > SEMANTIC_THRESHOLD {
					goodBattery++
				} else {
					badBattery++
				}
			}
		}
	}
}

//
func prodRanked(nlpComms []NlpComment) {
	for i := 0; i < len(nlpComms); i++ {
		var score float64
		//These are key attributes and are weighted more
		if containProCon(nlpComms[i].KeyPhrases) {
			score += KEY_ATTR_WEIGHT
		}

		//Other attributes get lower score
		if nlpComms[i].GoodBattery != 0 || nlpComms[i].BadBattery != 0 {
			score += MINOR_ATTR_WEIGHT
		}

		score += nlpComms[i].SentimentScore * 10

		nlpComms[i].NlpRankScore = score
	}
	// for _, comm := range nlpComms {
	// 	var score float64
	// 	//These are key attributes and are weighted more
	// 	if containProCon(comm.KeyPhrases) {
	// 		score += KEY_ATTR_WEIGHT
	// 	}
	//
	// 	//Other attributes get lower score
	// 	if comm.GoodBattery != 0 || comm.BadBattery != 0 {
	// 		score += MINOR_ATTR_WEIGHT
	// 	}
	//
	// 	score += comm.SentimentScore * 10
	// 	//the final computed score
	// 	comm.NlpRankScore = score
	// 	fmt.Println(comm.NlpRankScore)
	// }

	// prodRankMap := make(map[float64]NlpComment) //rankScore:comment HACK - assume unique
	// var scoreList []float64
	// for _, comm := range prodMap {
	// 	var rankScore float64 = 0
	// 	if containProCon(comm.KeyPhrases) {
	// 		rankScore = rankScore + 10
	// 	}
	//
	// 	if comm.GoodBattery != 0 {
	// 		rankScore = rankScore + 1
	// 	} else if comm.BadBattery != 0 {
	// 		rankScore = rankScore + 1
	// 	}
	//
	// 	rankScore = rankScore + (comm.SentimentScore * 10)
	//
	// 	comm.NlpRank = rankScore
	// 	prodRankMap[rankScore] = comm
	// 	scoreList = append(scoreList, rankScore)
	// }
	//
	// sort.Float64s(scoreList)
	// var rankedList []NlpComment
	// for i := len(scoreList) - 1; i >= 0; i-- {
	// 	rankedList = append(rankedList, prodRankMap[scoreList[i]])
	// }
	// return rankedList
}

//
// func rankComments(comments []NlpComment) {
// 	for _, c := range comments {
// 		if c.ReviewScore >= 4 {
// 			c.GoodComments = append(c.GoodComments, c.CommentText)
// 		} else {
// 			c.BadComments = append(c.BadComments, c.CommentText)
// 		}
// 	}
// }
//
func containProCon(phrases []string) bool {
	for _, p := range phrases {
		if strings.Contains(p, "pro") || strings.Contains(p, "pros") ||
			strings.Contains(p, "con") || strings.Contains(p, "cons") {
			return true
		}
	}
	return false
}

func sortComments(prod NlpProduct) ([]string, []string) {
	var goodComments []string
	var badComments []string
	for _, comm := range prod.Comments {
		if comm.CustomerRating > 4.0 {
			goodComments = append(goodComments, comm.CustomerComment)
		} else {
			badComments = append(badComments, comm.CustomerComment)
		}
	}
	return goodComments, badComments
}

//=============Structs================

//best buy comments
type BestBuyProd struct {
	Reviews []BestBuyReview `json:"reviews"`
}

type BestBuyReview struct {
	Rating  float64 `json:"rating"`
	Comment string  `json:"comment"`
}

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
	CustomerComment string
	CustomerRating  float64
	SentimentScore  float64
	KeyPhrases      []string
	GoodBattery     int
	BadBattery      int
	NlpRankScore    float64 //determins how high to place the comment
}

type NlpProduct struct {
	Comments     []NlpComment
	GoodComments []string
	BadComments  []string
}
