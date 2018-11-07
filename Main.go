package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
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

	res, _ := http.Get("https://speed.hetzner.de/100MB.bin")

	defer res.Body.Close()

	file := &File{
		r: res.Body,
		f: &drive.File{
			Name: "100MB.bin",
		},
		size: res.ContentLength,
		buf: make([]byte, 256 * 1024 * 4),
	}

	client.Timeout = time.Duration(30 * time.Second)

	_, err = file.ChunkedUpload(srv, client)
	if err != nil {
		log.Fatalf("Got error: %v", err)
	}
}


func (f *File) initUpload(client *http.Client) (string, error) {
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

func (f *File) uploadChunk(urls string, client *http.Client) (int64, error) {
	reader := &MyReader{
		buf: f.buf,
	}
	req, err := http.NewRequest("PUT", urls, reader)
	if err != nil {
		return 0, err
	}

	sending := fmt.Sprintf("bytes %v-%v/%v", f.pos, f.pos+int64(len(f.buf)-1), f.size)

	fmt.Printf("Sending %s\n", sending)
	req.Header.Set("Content-Range", sending)

	fmt.Println("Before request!")
	res, err := client.Do(req)
	fmt.Println("After request")
	if err != nil {
		return 0, err
	}

	defer googleapi.CloseBody(res)
	err = googleapi.CheckResponse(res)
	if res.StatusCode != 308 && err != nil {
		return 0, err
	}

	r, _ := regexp.Compile("bytes=\\d+-(\\d+)")
	resRange := res.Header.Get("Range")
	groups := r.FindStringSubmatch(resRange)
	if len(groups) < 2 {
		return 0, errors.New(fmt.Sprintf("response didn't include range header properly, heder is: %v", resRange))
	}

	ret, err := strconv.ParseInt(groups[1], 10, 64)
	if err != nil {
		return 0, err
	}

	return ret , nil
}

func (f *File) fillBuf() (bool, error){
	pos := 0
	for pos < len(f.buf){
		off, err := f.r.Read(f.buf[pos:])
		if err == io.EOF {
			//TODO check later...
			f.buf = f.buf[:pos+off]
			return false, nil
		}
		if err != nil {
			return false, err
		}
		pos += off
	}
	return true, nil
}

func (f *File) ChunkedUpload(srv *drive.Service, client *http.Client) (string, error)  {
	fmt.Printf("Timeout: %v\n", client.Timeout)
	urls, err := f.initUpload(client)
	if err != nil {
		return "", err
	}

	fmt.Printf("I got an upload url: %s\n", urls)

	for {
		more, err := f.fillBuf()
		if err != nil {
			return "", err
		}
		fmt.Println("Before upload Chunk")
		read, err := f.uploadChunk(urls, client)
		if err != nil {
			return "", err
		}
		fmt.Println("After upload Chunk")
		if read != f.pos + int64(len(f.buf)-1) {
			return "", errors.New(fmt.Sprintf("wrong return size, expected %v got %v", f.pos + int64(len(f.buf)), read))
		}
		f.pos = read + 1
		if !more {
			break
		}
	}

	return "", nil
}

type MyReader struct {
	r io.Reader
	buf []byte
	pos int
}

/* func (r *MyReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart: {}
	case io.SeekCurrent: {}
	case io.SeekEnd: {}
	}

	return 0, nil
} */

func (r *MyReader) Read(p []byte) (n int, err error) {
	n = copy(p, r.buf[r.pos:])
	r.pos += n
	if r.pos == len(r.buf) {
		return n, io.EOF
	}
	return n, nil
}

type File struct {
	r io.Reader
	f *drive.File
	size int64
	pos int64
	buf []byte
}

type Host interface {
	Login(usr string, pwd string) bool
	File(url string, pwd string) File
	Folder(url string) (urls []string)
}