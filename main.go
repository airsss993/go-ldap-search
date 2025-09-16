package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/joho/godotenv"
)

type Person struct {
	DN         string            `json:"dn"`
	Attributes map[string]string `json:"attributes"`
}

type SearchResponse struct {
	Total  int      `json:"total"`
	People []Person `json:"people"`
	Error  string   `json:"error,omitempty"`
}

func searchPeopleInOU(host string, port string, DN string) (*SearchResponse, error) {
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("invalid port format: %w", err)
	}

	conn, err := ldap.DialURL(fmt.Sprintf("ldap://%s:%v", host, portInt))
	if err != nil {
		return nil, fmt.Errorf("connection error: %w", err)
	}
	defer conn.Close()

	conn.SetTimeout(5 * time.Second)

	err = conn.UnauthenticatedBind("")
	if err != nil {
		return nil, fmt.Errorf("bind error: %w", err)
	}

	searchRequest := ldap.NewSearchRequest(
		DN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		"(objectClass=person)",
		[]string{"*"},
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("search error: %w", err)
	}

	response := &SearchResponse{
		Total:  len(sr.Entries),
		People: make([]Person, len(sr.Entries)),
	}

	for i, entry := range sr.Entries {
		person := Person{
			DN:         entry.DN,
			Attributes: make(map[string]string),
		}

		for _, attr := range entry.Attributes {
			if len(attr.Values) > 0 && attr.Name != "dn" {
				person.Attributes[attr.Name] = attr.Values[0]
			}
		}

		response.People[i] = person
	}

	return response, nil
}

func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	host := os.Getenv("HOST")
	port := os.Getenv("PORT")
	DN := os.Getenv("DN")

	response, err := searchPeopleInOU(host, port, DN)
	if err != nil {
		response = &SearchResponse{
			Error: err.Error(),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func staticHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	http.HandleFunc("/", staticHandler)
	http.HandleFunc("/api/search", searchHandler)

	port := ":8080"
	fmt.Printf("Server starting on http://localhost%s\n", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
