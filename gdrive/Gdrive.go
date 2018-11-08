package gdrive

import (
	"errors"
	"fmt"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
)

func Create(client *http.Client, srv *drive.Service) (*Gdrive) {
	return &Gdrive{
		client:client,
		srv:srv,
	}
}

func (g *Gdrive) Upload(r io.Reader, f *drive.File, size int64) (*drive.File, error) {
	ResumableUpload{
		g: 	g,
		r:    r,
		f:    f,
		size: size,
		buf:  make([]byte, 256*1024*4),
	}.chunkedUpload()

	return &drive.File{}, nil
}

type ResumableUpload struct {
	g *Gdrive
	r    io.Reader
	f    *drive.File
	size int64
	pos  int64
	buf  []byte
}

type MyReader struct {
	r   io.Reader
	buf []byte
	pos int
}

type Gdrive struct {
	client *http.Client
	srv *drive.Service
}

func (r *MyReader) Read(p []byte) (n int, err error) {
	n = copy(p, r.buf[r.pos:])
	r.pos += n
	if r.pos == len(r.buf) {
		return n, io.EOF
	}
	return n, nil
}

func (rx *ResumableUpload) initUpload(client *http.Client) (string, error) {
	//URL query parameters
	query := url.Values{
		"uploadType":         {"resumable"},
		"supportsTeamDrives": {"true"},
	}

	//Base url
	urls := "https://www.googleapis.com/upload/drive/v3/files"

	//Let's add all query parameters to the url
	urls += "?" + query.Encode()

	//Create a body from the file
	body, err := googleapi.WithoutDataWrapper.JSONReader(rx.f)
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
	req.Header.Set("X-Upload-Content-Length", fmt.Sprintf("%v", rx.size))

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

func (rx *ResumableUpload) uploadChunk(urls string) (StatusCode int, Range int64, Error error) {
	reader := &MyReader{
		buf: rx.buf,
	}
	req, err := http.NewRequest("PUT", urls, reader)
	if err != nil {
		return -1, 0, err
	}

	sending := fmt.Sprintf("bytes %v-%v/%v", rx.pos, rx.pos+int64(len(rx.buf)-1), rx.size)

	fmt.Printf("Sending %s\n", sending)
	req.Header.Set("Content-Range", sending)

	fmt.Println("Before request!")
	res, err := rx.g.client.Do(req)
	fmt.Println("After request")
	if err != nil {
		return -2, 0, err
	}

	//TODO do proper parsing, 308 means everything is fine send more data 5XX means I need retry, 201 or 200 means upload done.
	defer googleapi.CloseBody(res)

	switch res.StatusCode {
	//Resume incomplete
	case 308:
		r, _ := regexp.Compile("bytes=\\d+-(\\d+)")
		resRange := res.Header.Get("Range")
		groups := r.FindStringSubmatch(resRange)
		if len(groups) < 2 {
			return 308, 0, errors.New(fmt.Sprintf("response didn't include range header properly, heder is: %v", resRange))
		}

		ret, err := strconv.ParseInt(groups[1], 10, 64)
		if err != nil {
			return 308, 0, err
		}

		return 308, ret, nil
		//Everything went okay

	case 200:
		//TODO make this also return the created file or something... but I need to make it nicer than it is now
		fallthrough
	case 201:
		return res.StatusCode, -1, nil
	default:
		err = googleapi.CheckResponse(res)
		if err != nil {
			return res.StatusCode, 0, err
		}
		return res.StatusCode, 0, nil
	}
}

func (rx *ResumableUpload) fillBuf() (bool, error) {
	pos := 0
	for pos < len(rx.buf) {
		off, err := rx.r.Read(rx.buf[pos:])
		if err == io.EOF {
			rx.buf = rx.buf[:pos+off]
			return false, nil
		}
		if err != nil {
			return false, err
		}
		pos += off
	}
	return true, nil
}

//TODO deal with upload interrupted, so 5XX errors, also ratelimitng?
func (rx *ResumableUpload) chunkedUpload() (string, error) {
	fmt.Printf("Timeout: %v\n", rx.g.client.Timeout)
	urls, err := rx.initUpload(rx.g.client)
	if err != nil {
		return "", err
	}

	fmt.Printf("I got an upload url: %s\n", urls)

	for {
		more, err := rx.fillBuf()
		if err != nil {
			return "", err
		}
		status, read, err := rx.uploadChunk(urls)
		if status == 200 || status == 201 {
			fmt.Printf("Upload done!\n")
			break
		}
		if err != nil {
			return "", err
		}
		if read != rx.pos+int64(len(rx.buf)-1) {
			return "", errors.New(fmt.Sprintf("wrong return size, expected %v got %v", rx.pos+int64(len(rx.buf)), read))
		}
		rx.pos = read + 1
		if !more {
			break
		}
	}

	return "", nil
}
