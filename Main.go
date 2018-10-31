package main

import (
"encoding/json"
"fmt"
"io/ioutil"
"log"
"net/http"
	"net/http/cookiejar"
	"os"

"golang.org/x/net/context"
"golang.org/x/oauth2"
"golang.org/x/oauth2/google"
"google.golang.org/api/drive/v3"
)

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	tokenFile := "secret/token.json"
	tok, err := tokenFromFile(tokenFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokenFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func main() {
	b, err := ioutil.ReadFile("secret/credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	srv, err := drive.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Drive client: %v", err)
	}

	rsp, err := srv.Files.Get("root").Do()
	if err != nil {
		log.Fatalf("Well that went badly: %v", err)
	}
	cookiejar.New(nil)
	fmt.Println(rsp.Id)

	r, err := srv.Files.List().
		SupportsTeamDrives(true).
		IncludeTeamDriveItems(true).
		Q("mimeType='application/vnd.google-apps.folder'").
		PageSize(1000).Fields("files(id, name, parents)").Do()
	if err != nil {
		log.Fatalf("Unable to retrieve files: %v", err)
	}
	fmt.Println("Files:")
	if len(r.Files) == 0 {
		fmt.Println("No files found.")
	} else {
		for _, i := range r.Files {
			fmt.Printf("%s (%s)\n", i.Name, i.Id)
		}
	}

	fmt.Println("Trying to upload a thing!")

	response, err := http.Get("https://speed.hetzner.de/100MB.bin")
	if err != nil {
		log.Fatalf("Something went very wrong %v", err)
	}
	defer response.Body.Close()

	file := drive.File{
		Name: "100MB.bin",
		TeamDriveId: "0AGZqTq8YYmQ7Uk9PVA",
		Parents: []string{"0AGZqTq8YYmQ7Uk9PVA"},
	}

	created, err := srv.Files.Create(&file).Media(response.Body).SupportsTeamDrives(true).Do()
	if err != nil {
		log.Fatalf("Well this is akward: %v", err)
	}

	fmt.Printf("The file has the id %s", created.Id)
}