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
	OU         string            `json:"ou"`
	Members    []string          `json:"members,omitempty"`
}

type OUData struct {
	Name   string   `json:"name"`
	Total  int      `json:"total"`
	People []Person `json:"people"`
}

type SearchResponse struct {
	Total int               `json:"total"`
	OUs   map[string]OUData `json:"ous"`
	Error string            `json:"error,omitempty"`
}

func searchPeopleInOU(host string, port string, baseDN string, ouName string) ([]Person, error) {
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

	// Выбираем фильтр в зависимости от OU
	var filter string
	switch ouName {
	case "groups":

		filter = "(|(objectClass=group)(objectClass=groupOfNames)(objectClass=posixGroup))"
	default:
		filter = "(objectClass=person)"
	}

	searchRequest := ldap.NewSearchRequest(
		fmt.Sprintf("ou=%s,%s", ouName, baseDN),
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		filter,
		[]string{"*"},
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("search error in OU %s: %w", ouName, err)
	}

	fmt.Printf("Debug: OU %s found %d entries with filter %s\n", ouName, len(sr.Entries), filter)

	people := make([]Person, len(sr.Entries))

	for i, entry := range sr.Entries {
		person := Person{
			DN:         entry.DN,
			OU:         ouName,
			Attributes: make(map[string]string),
		}

		for _, attr := range entry.Attributes {
			if len(attr.Values) > 0 && attr.Name != "dn" {
				// Для групп сохраняем членов отдельно
				if (attr.Name == "member" || attr.Name == "memberUid") && ouName == "groups" {
					person.Members = attr.Values
				} else {
					person.Attributes[attr.Name] = attr.Values[0]
				}
			}
		}

		people[i] = person
	}

	return people, nil
}

func searchAllOUs(host string, port string, baseDN string) (*SearchResponse, error) {
	ous := []string{"people", "teachers", "groups"}

	response := &SearchResponse{
		Total: 0,
		OUs:   make(map[string]OUData),
	}

	for _, ouName := range ous {
		people, err := searchPeopleInOU(host, port, baseDN, ouName)
		if err != nil {
			fmt.Printf("Warning: Could not search OU %s: %v\n", ouName, err)
			continue
		}

		response.OUs[ouName] = OUData{
			Name:   ouName,
			Total:  len(people),
			People: people,
		}
		response.Total += len(people)
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
	baseDN := "dc=it-college,dc=ru"

	response, err := searchAllOUs(host, port, baseDN)
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
