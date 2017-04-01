package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

func main() {
	fmt.Println("Start server")
	server := mux.NewRouter().StrictSlash(true)
	server.Headers("Content-Type", "application/json",
		"Ocp-Apim-Subscription-Key", "9122668a88454ac9bed0b816edbe5c8c",
		"Accept", "application/json")
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
	prodId, err := strconv.Atoi(prodIdRaw)
	if err != nil {
		fmt.Println("Product num", prodIdRaw, "error:", err)
		fmt.Fprintf(w, "Could not read product", prodIdRaw)
		return
	}
	fmt.Println("Product received", prodId)
	fmt.Fprintf(w, "Product received %s", prodIdRaw)
}

func Dev(w http.ResponseWriter, req *http.Request) {
	resp, err := httpGet("http://www.bestbuy.ca/api/v2/json/reviews/10415309?page=1&pagesize=20&source=us")
	if err != nil {
		fmt.Println("GET error:", err)
		return
	}
	//all good in the hood
	if resp.StatusCode >= 200 && resp.StatusCode <= 300 {
		//get the json response body as []byte
		respBodyBytes, _ := ioutil.ReadAll(resp.Body)

		//get the comments as []string
		comments := getCommentsFromResp(respBodyBytes)

		//json package for microsoft cog labs
		microsoftJson := parseCommentsForMicrosoft(comments)

		fmt.Println(string(microsoftJson))

	}
}

//=============Helpers==================

func getCommentsFromResp(respBodyBytes []byte) []string {
	var dat map[string]interface{}
	if err := json.Unmarshal(respBodyBytes, &dat); err != nil {
		panic(err)
		return nil
	}

	var comments []string
	for _, val := range dat["reviews"].([]interface{}) {
		v := val.(map[string]interface{})
		for k2, v2 := range v {
			if k2 == "comment" {
				comments = append(comments, v2.(string))
			}
			fmt.Println(k2, ":=:", v2)
		}
		fmt.Println("\n\n")
	}
	return comments
}

func parseCommentsForMicrosoft(comments []string) []byte {
	//TODO: stop if ID>100
	idCount := 1
	var d Document
	for _, comment := range comments {
		commElement := DocElement{Language: "en", Id: strconv.Itoa(idCount), Text: comment}
		d.Documents = append(d.Documents, commElement)
		idCount++
	}
	j, err := json.Marshal(d)
	if err != nil {
		fmt.Println("json marshal error", err)
		return nil
	}
	return j
}

func httpGet(url string) (*http.Response, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET error", err)
	}
	return resp, nil
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
