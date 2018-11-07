package main

import (
	"encoding/json"
	"fmt"
	"google.golang.org/api/googleapi"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
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

	ChunckedUpload(&File{
		r: nil,
		f: &drive.File{
			Name: "Test",
		},
		size: int64(10),
	}, srv, client)
}


func initUpload(f *File, client *http.Client) (string, error) {
	//URL query parameters
	query := url.Values{
		"uploadType": {"resumable"},
		"supportsTeamDrives": {"true"},
	}

	//Base url
	urls := "https://www.googleapis.com/upload/drive/v3/files"

	//Let's add all query parameters to the url
	urls += "?" + query.Encode()

	//Create a body from the file
	body, err := googleapi.WithoutDataWrapper.JSONReader(f.f)
	if err != nil {
		return "", err
	}

	//New request
	req, err := http.NewRequest("POST", urls, body)
	if err != nil {
		return "", err
	}

	//Let's be nice and tell google how big the file we'll upload is.
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Length", fmt.Sprintf("%v", f.size))

	//And let's finally do the request to get the actual URL that we'll use to upload things.
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	err = googleapi.CheckResponse(res)
	defer googleapi.CloseBody(res)
	if err != nil {
		return "", err
	}

	return res.Header.Get("Location"), nil
}

func ChunckedUpload(f *File, srv *drive.Service, client *http.Client) (string, error)  {


	url, err := initUpload(f, client)
	if err != nil {
		return "", err
	}

	fmt.Printf("I got an upload url: %s", url)

	/**
	reader := &MyReader{
		r: f.r,
		buf: make([]byte, 256 * 1024), //Currently just using min size according to: https://developers.google.com/drive/api/v3/resumable-upload
	}

	send := int64(0)

	for send < f.size {
		req, err := http.NewRequest("PUT", urls, reader)
		if err != nil {
			log.Fatalf("Well that didn't go as expected: %v", err)
		}
		req.Header.Set("Content-Range", fmt.Sprintf("%v-%v/%v", send, f.size-send, f.size))
		res, err := client.Do(req)
		if err != nil {
			log.Fatalf("Request didn't go as expected: %v", err)
		}
		err = googleapi.CheckResponse(res)

		if err != nil {
			log.Fatalf("Why you hate me google? %v", err)
		}

	}**/

	return "", nil
}

type MyReader struct {
	r io.Reader
	buf []byte
	pos int
}

func (r *MyReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart: {}
	case io.SeekCurrent: {}
	case io.SeekEnd: {}
	}

	return 0, nil
}

func (r *MyReader) Read(p []byte) (n int, err error) {
	n = copy(p, r.buf[r.pos:])
	r.pos += n
	return n, nil
}

type File struct {
	r io.Reader
	f *drive.File
	size int64
}

type Host interface {
	Login(usr string, pwd string) bool
	File(url string, pwd string) File
	Folder(url string) (urls []string)
}