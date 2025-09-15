package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/joho/godotenv"
)

func searchPeopleInOU(host string, port string, DN string) error {
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port format: %w", err)
	}

	conn, err := ldap.DialURL(fmt.Sprintf("ldap://%s:%v", host, portInt))
	if err != nil {
		return fmt.Errorf("connection error: %w", err)
	}
	defer conn.Close()

	conn.SetTimeout(5 * time.Second)

	err = conn.UnauthenticatedBind("")
	if err != nil {
		return fmt.Errorf("bind error: %w", err)
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
		return fmt.Errorf("search error: %w", err)
	}

	totalPeople := len(sr.Entries)
	fmt.Printf("=== Search results in %s ===\n", DN)
	fmt.Printf("Total number of people: %d\n\n", totalPeople)

	if totalPeople == 0 {
		fmt.Println("No people found")
		return nil
	}

	fmt.Printf("First 10 people:\n")
	for i := 0; i < 10; i++ {
		entry := sr.Entries[i]
		fmt.Printf("\n%d. DN: %s\n", i+1, entry.DN)

		for _, attr := range entry.Attributes {
			if len(attr.Values) > 0 && attr.Name != "dn" {
				fmt.Printf("   %s: %s\n", attr.Name, attr.Values[0])
			}
		}
	}

	if totalPeople > 10 {
		fmt.Printf("\n... and %d more people\n", totalPeople-10)
	}

	return nil
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	host := os.Getenv("HOST")
	port := os.Getenv("PORT")
	DN := os.Getenv("DN")

	err = searchPeopleInOU(host, port, DN)
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
}
