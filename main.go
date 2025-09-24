package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
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

type UserGroupsResponse struct {
	UID    string   `json:"uid"`
	Groups []Person `json:"groups"`
	Total  int      `json:"total"`
	Error  string   `json:"error,omitempty"`
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

func findUserGroups(host string, port string, baseDN string, uid string) (*UserGroupsResponse, error) {
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

	// Поиск всех групп в OU groups
	searchRequest := ldap.NewSearchRequest(
		fmt.Sprintf("ou=groups,%s", baseDN),
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		"(|(objectClass=group)(objectClass=groupOfNames)(objectClass=posixGroup))",
		[]string{"*"},
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("search error: %w", err)
	}

	response := &UserGroupsResponse{
		UID:    uid,
		Groups: []Person{},
		Total:  0,
	}

	// Проверяем членство пользователя в каждой группе
	for _, entry := range sr.Entries {
		isMember := false

		// Проверяем атрибут member (полный DN)
		for _, attr := range entry.Attributes {
			if attr.Name == "member" {
				for _, memberDN := range attr.Values {
					// Проверяем, содержит ли DN пользователя uid
					if containsUID(memberDN, uid) {
						isMember = true
						break
					}
				}
			}
			// Проверяем атрибут memberUid (только uid)
			if attr.Name == "memberUid" {
				for _, memberUID := range attr.Values {
					if memberUID == uid {
						isMember = true
						break
					}
				}
			}
		}

		if isMember {
			group := Person{
				DN:         entry.DN,
				OU:         "groups",
				Attributes: make(map[string]string),
			}

			for _, attr := range entry.Attributes {
				if len(attr.Values) > 0 && attr.Name != "dn" {
					if attr.Name == "member" || attr.Name == "memberUid" {
						group.Members = attr.Values
					} else {
						group.Attributes[attr.Name] = attr.Values[0]
					}
				}
			}

			response.Groups = append(response.Groups, group)
			response.Total++
		}
	}

	return response, nil
}

// Вспомогательная функция для проверки наличия uid в DN
func containsUID(dn string, uid string) bool {
	// Проверяем различные форматы DN
	// Например: "uid=student1,ou=people,dc=it-college,dc=ru"
	// или "cn=Student Name,ou=people,dc=it-college,dc=ru"

	uidPattern := fmt.Sprintf("uid=%s,", uid)
	cnPattern := fmt.Sprintf("cn=%s,", uid)

	// Проверяем наличие uid= или cn= с данным значением
	return strings.Contains(strings.ToLower(dn), strings.ToLower(uidPattern)) ||
	       strings.Contains(strings.ToLower(dn), strings.ToLower(cnPattern))
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

func userGroupsHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uid := r.URL.Query().Get("uid")
	if uid == "" {
		response := &UserGroupsResponse{
			Error: "uid parameter is required",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	host := os.Getenv("HOST")
	port := os.Getenv("PORT")
	baseDN := "dc=it-college,dc=ru"

	response, err := findUserGroups(host, port, baseDN, uid)
	if err != nil {
		response = &UserGroupsResponse{
			UID:   uid,
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
	http.HandleFunc("/api/user-groups", userGroupsHandler)

	port := ":8080"
	fmt.Printf("Server starting on http://localhost%s\n", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
