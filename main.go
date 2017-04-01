package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

func main() {
	fmt.Println("Start server")
	server := mux.NewRouter().StrictSlash(true)
	server.HandleFunc("/", Index).Methods("GET")
	server.HandleFunc("/hello/{name}", Hello)
	server.HandleFunc("/product/{prodId}", CommentNLP).Methods("GET")
	// http.Handle("/", server)
	log.Fatal(http.ListenAndServe(":8080", server))
}

func Index(w http.ResponseWriter, req *http.Request) {
	fmt.Println("Index path")
	fmt.Fprintf(w, "Index")
}

func Hello(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	name := vars["name"]
	fmt.Println("Name:", name)
	fmt.Fprintf(w, "Hello %s", name)
}

func HelpRoute(w http.ResponseWriter, req *http.Request) {
	fmt.Println("Help Path")
	fmt.Fprintf(w, "Help please!")
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
