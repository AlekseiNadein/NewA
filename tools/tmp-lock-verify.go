//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

func tryLogin(client *http.Client, base string, creds map[string]string) bool {
	body, _ := json.Marshal(creds)
	req, _ := http.NewRequest(http.MethodPost, base+"/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	fmt.Printf("login %q -> %d %s\n", creds["name"], resp.StatusCode, string(b))
	return resp.StatusCode == 200
}

func main() {
	base := "http://localhost:8080"
	estID := "est_1b0e1363326ce2ab"
	attempts := []map[string]string{
		{
			"companyName": "\u041e\u041e\u041e \u041d\u041f\u041f \"\u0410\u0412\u0421-\u041d\"",
			"name":        "\u0428\u0442\u0430\u0439\u0433\u0435\u0440 \u0410. \u0424.",
			"password":    "admin123",
		},
		{
			"companyName": "\u041e\u041e\u041e \u041d\u041f\u041f \"\u0410\u0412\u0421-\u041d\"",
			"name":        "\u0428\u0442\u0430\u0439\u0433\u0435\u0440 \u0410.\u0424.",
			"password":    "admin123",
		},
		{
			"companyName": "",
			"name":        "nadein.av@yandex.ru",
			"password":    "admin123",
		},
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 2 * time.Minute}
	ok := false
	for _, creds := range attempts {
		jar, _ = cookiejar.New(nil)
		client.Jar = jar
		if tryLogin(client, base, creds) {
			ok = true
			break
		}
	}
	if !ok {
		fmt.Println("all logins failed")
		return
	}
	for _, step := range []struct{ method, path string }{
		{http.MethodPut, "/api/estimates/" + estID + "/lock"},
		{http.MethodDelete, "/api/estimates/" + estID + "/lock"},
	} {
		req, _ := http.NewRequest(step.method, base+step.path, nil)
		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("%s %s -> %d %s\n", step.method, step.path, resp.StatusCode, string(b))
	}
	fmt.Println("waiting 5s...")
	time.Sleep(5 * time.Second)
}
