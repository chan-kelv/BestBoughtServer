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
	server.HandleFunc("/", Index).Methods("GET")
	server.HandleFunc("/help", HelpRoute).Methods("GET")
	server.HandleFunc("/product/{prodId}", CommentNLP).Methods("GET") //main use
	log.Fatal(http.ListenAndServe(":8080", server))
}

//================Routes============

//Says hello! used as test route
func Index(w http.ResponseWriter, req *http.Request) {
	fmt.Println("Hello there")
	fmt.Fprintf(w, "Hello there!")
}

//another test function
func HelpRoute(w http.ResponseWriter, req *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")
	resp := "Active routes \n\n/product{BestBuyProductId}"
	fmt.Fprintf(w, resp)
}

/*
	Given a best buy product id, this route uses best buy api to return a list
	of the comments for the product. Stores each comment into NlpComment and runs
	them through Microsoft Cog systems semantic and keyword analysis.
	The comments are assigned a score based on their cog system scores and keyword attributes
	(self determined).
	The comments are then ranked based on the score and returned as a json package to the user
	TODO comments restricted to 50 for hack
*/
func CommentNLP(w http.ResponseWriter, req *http.Request) {
	//get the product id from the url
	vars := mux.Vars(req)
	prodIdRaw := vars["prodId"]

	//best buy api for product info
	resp, err := httpGet("http://www.bestbuy.ca/api/v2/json/reviews/" + prodIdRaw + "?page=1&pagesize=50&source=us")
	if err != nil {
		fmt.Println("GET error:", err)
		return
	}

	//all good in the hood
	if resp.StatusCode >= 200 && resp.StatusCode <= 300 {
		//get the json response body as []byte
		respBodyBytes, _ := ioutil.ReadAll(resp.Body)

		//get the comments as []NlpComment
		commStructs := getCommentsFromResp(respBodyBytes)

		// //json package for microsoft cog labs turns commStruct into []byte
		microsoftJson := parseCommentsForMicrosoft(commStructs)

		/*
			Library parsing TODO add more libraies
		*/
		// //call sentiment analysis
		microsoftSentiment(microsoftJson, commStructs)
		//call keyword analysis
		microsoftKeyWords(microsoftJson, commStructs)

		/*
			Analyze key words here TODO add more attributes
		*/
		// nlp comment is now ready to parse for key words
		batteryWordCount(commStructs)

		//Give the comments a final score based on attributes and semantics
		prodRanked(commStructs)

		//rank the comm structs with the new nlp score
		sort.Slice(commStructs, func(i int, j int) bool {
			return commStructs[i].NlpRankScore > commStructs[j].NlpRankScore //from highest to lowest
		})

		//create nlpProd for this best buy item
		nlpProduct := NlpProduct{Comments: commStructs}

		//Sort the comments based on initial customer rating
		goodComm, badComm := sortComments(nlpProduct)
		nlpProduct.GoodComments = goodComm
		nlpProduct.BadComments = badComm

		//encode the final nlp product for return
		final, err := json.Marshal(nlpProduct)
		if err != nil {
			fmt.Println("final marshall error:", err)
		}

		//return as text to client
		fmt.Fprintf(w, string(final))
		fmt.Println("Product retrieved:", prodIdRaw)
	}
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
	//deserialize back into BestBuyProd
	var bbData BestBuyProd
	if err := json.Unmarshal(respBodyBytes, &bbData); err != nil {
		panic(err)
	}

	//add relavent fields into nlp comments (comment text, rating)
	var nlpComms []NlpComment
	for _, comment := range bbData.Reviews {
		nlpComm := NlpComment{CustomerComment: comment.Comment, CustomerRating: comment.Rating}
		nlpComms = append(nlpComms, nlpComm)
	}

	return nlpComms
}

//Turn comments into []byte for microsoft to process
func parseCommentsForMicrosoft(comments []NlpComment) []byte {
	idCount := 1 //microsoft needs an id to start at 1 for api

	//put all comments into document to be processed as json arr
	var d Document
	for _, comm := range comments {
		commElement := DocElement{Language: "en", Id: strconv.Itoa(idCount), Text: comm.CustomerComment}
		d.Documents = append(d.Documents, commElement)
		idCount++
	}

	//serialize into []byte
	j, err := json.Marshal(d)
	if err != nil {
		fmt.Println("json marshal error", err)
		return nil
	}
	return j
}

//send comments to microsoft semantics to give each a score
//TODO can only take 100 in a package
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

	//for each sentimate store, add it to the nlpcomment
	for i := 0; i < len(nlpComm); i++ {
		nlpComm[i].SentimentScore = sentResp.Documents[i].Score
	}
}

//Give comments to microsoft api to id key words in each comment
//TODO can only take 100 in a package
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

	var keyResp KeywordResponse
	err = json.Unmarshal(body, &keyResp)
	if err != nil {
		fmt.Println("JSON unmarshall error:", err)
		return
	}

	//add key words to nlpcomment
	for i := 0; i < len(nlpComm); i++ {
		nlpComm[i].KeyPhrases = keyResp.Documents[i].KeyPhrases
	}
}

//determins if the keywords contains an attribute word:battery
//if semantics is > threshold, consider the comment +1 for good battery else badbattery
func batteryWordCount(nlpComms []NlpComment) {
	//for each comment
	for i := 0; i < len(nlpComms); i++ {
		comm := nlpComms[i]
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
		nlpComms[i].GoodBattery = goodBattery
		nlpComms[i].BadBattery = badBattery
	}
}

/* Gives the comments a final nlp score to find most relavent comments
 Algorithm so far is:
	Score = KEY Attributes * 10  +
					MINOR Attributes * 1 +
					Semantic Score * 10
*/

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
}

//KEY ATTRIBUTE: if a comment has "pro" "con" list
func containProCon(phrases []string) bool {
	for _, phrase := range phrases {
		p := strings.ToLower(phrase)
		if strings.Contains(p, "pro") || strings.Contains(p, "pros") ||
			strings.Contains(p, "con") || strings.Contains(p, "cons") {
			return true
		}
	}
	return false
}

/*
	prod is assumed to have sorted comments at this point
	This method takes the comments and looks at each of the comments inital best buy
	customer rating. If the customer rating is > 4.0 it is considered a good comment
	(used in the display comments section). This ensures that the two lists of good
	and bad comments show only the most high scored nlp comments first as we deem them most useful
*/
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
