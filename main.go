package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var tpl *template.Template

var HTTPClient = http.Client{
	Timeout: 30 * time.Second,
}

type WikipediaSearchResponse struct {
	BatchComplete string `json:"batchcomplete"`
	Continue      struct {
		Sroffset int    `json:"sroffset"`
		Continue string `json:"continue"`
	} `json:"continue"`
	Query struct {
		SearchInfo struct {
			TotalHits int `json:"totalhits"`
		} `json:"searchinfo"`
		Search []struct {
			Ns        int       `json:"ns"`
			Title     string    `json:"title"`
			PageID    int       `json:"pageid"`
			Size      int       `json:"size"`
			WordCount int       `json:"wordcount"`
			Snippet   string    `json:"snippet"`
			Timestamp time.Time `json:"timestamp"`
		} `json:"search"`
	} `json:"query"`
}

type Search struct {
	Query      string
	TotalPages int
	NextPage   int
	Results    *WikipediaSearchResponse
}

func (s *Search) IsLastPage() bool {
	return s.NextPage >= s.TotalPages
}

func (s *Search) CurrentPage() int {
	if s.NextPage == 1 {
		return s.NextPage
	}

	return s.NextPage - 1
}

func (s *Search) PreviousPage() int {
	return s.CurrentPage() - 1
}

func indexHandler(w http.ResponseWriter, r *http.Request) error {
	buf := &bytes.Buffer{}
	err := tpl.Execute(buf, nil)
	if err != nil {
		return err
	}

	_, err = buf.WriteTo(w)

	return err
}

func searchWikipedia(
	searchQuery string,
	pageSize, resultsOffset int,
) (*WikipediaSearchResponse, error) {
	resp, err := HTTPClient.Get(
		fmt.Sprintf(
			"https://en.wikipedia.org/w/api.php?action=query&list=search&prop=info&inprop=url&utf8=&format=json&origin=*&srlimit=%d&srsearch=%s&sroffset=%d",
			pageSize,
			searchQuery,
			resultsOffset,
		),
	)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var searchResponse WikipediaSearchResponse

	err = json.Unmarshal(body, &searchResponse)
	if err != nil {
		return nil, err
	}

	return &searchResponse, nil
}

func searchHandler(w http.ResponseWriter, r *http.Request) error {
	u, err := url.Parse(r.URL.String())
	if err != nil {
		return err
	}

	params := u.Query()
	searchQuery := params.Get("q")
	pageNum := params.Get("page")
	if pageNum == "" {
		pageNum = "1"
	}

	nextPage, err := strconv.Atoi(pageNum)
	if err != nil {
		return err
	}

	pageSize := 20

	resultsOffset := (nextPage - 1) * pageSize

	searchResponse, err := searchWikipedia(searchQuery, pageSize, resultsOffset)
	if err != nil {
		return err
	}

	totalHits := searchResponse.Query.SearchInfo.TotalHits

	search := &Search{
		Query:      searchQuery,
		Results:    searchResponse,
		TotalPages: int(math.Ceil(float64(totalHits) / float64(pageSize))),
		NextPage:   nextPage + 1,
	}

	buf := &bytes.Buffer{}
	err = tpl.Execute(buf, search)
	if err != nil {
		return err
	}

	_, err = buf.WriteTo(w)

	return err
}

type handlerWithError func(w http.ResponseWriter, r *http.Request) error

func (fn handlerWithError) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := fn(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func htmlSafe(str string) template.HTML {
	return template.HTML(str)
}

func init() {
	var err error

	tpl, err = template.New("index.html").Funcs(template.FuncMap{
		"htmlSafe": htmlSafe,
	}).ParseFiles("index.html")
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	fs := http.FileServer(http.Dir("assets"))

	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets/", fs))
	mux.Handle("/search", handlerWithError(searchHandler))
	mux.Handle("/", handlerWithError(indexHandler))

	log.Fatal(http.ListenAndServe(":3000", mux))
}
