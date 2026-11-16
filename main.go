package main

import (
	"log/slog"

	"bufio"
	"bytes"
	"cmp"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"encoding/json"
	"html/template"
	"io/fs"
	"net/url"
	"path/filepath"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"

	gmhtml5 "github.com/mwyvr/goldmark-html5"
	admonitions "github.com/stefanfritsch/goldmark-admonitions"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
)

type Config struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	FullURL     string `json:"full_url"`
	Author      string `json:"author"`
	AuthorURL   string `json:"author_url"`
	Via         string `json:"via"`
	PrettyURLs  bool   `json:"pretty_urls"`
	PageSize    int    `json:"page_size"`
}

type Article struct {
	Fname       string
	Title       string
	Timestamp   time.Time
	Date        string
	Time        string
	Description string
	Category    string
	Preview     template.HTML
	Body        template.HTML
	Cut         bool
	Image       string
	URL         string
	ShareURL    string
	Author      string
	AuthorURL   string
}

type PageContext struct {
	Title       string
	Description string
	Image       string
	Cfg         *Config
	Articles    []*Article
	Categories  []string
	InArticle   bool
	CurrentPage int
	TotalPages  int
	Pages       []string
}

const Version = "v0.5"
const PagesDir = "pages"
const OutDir = "out"

var articles []*Article
var categories map[string][]*Article
var categoryTitles []string
var cfg Config
var logger *slog.Logger
var tmpl *template.Template
var md goldmark.Markdown

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func ToHTML(input []byte) template.HTML {
	var buf bytes.Buffer

	err := md.Convert(input, &buf)
	check(err)

	return template.HTML(string(buf.Bytes()))
}

func ArrangeArticles() {
	// Sort articles by timestamp
	slices.SortFunc(articles, func(a, b *Article) int {
		return b.Timestamp.Compare(a.Timestamp)
	})

	// Arrange into categories
	for _, obj := range articles {
		AddToCategory(obj.Category, obj)
	}
}
func AddToCategory(cat string, obj *Article) {
	categories[cat] = append(categories[cat], obj)
}

func SortCategories() {
	// Sorts category titles by the amount of articles per category
	categoryTitles = make([]string, 0, len(categories))
	for k := range categories {
		categoryTitles = append(categoryTitles, k)
	}

	slices.SortFunc(categoryTitles, func(a, b string) int {
		return cmp.Compare(len(categories[b]), len(categories[a]))
	})
}

func CreateShareURL(obj *Article) string {
	params := url.Values{}
	params.Add("url", cfg.FullURL+obj.URL)
	params.Add("via", cfg.Via)
	params.Add("text", obj.Title)
	return fmt.Sprintf("https://twitter.com/intent/tweet?%s", params.Encode())
}

func ParseArticle(path string, d fs.DirEntry, e error) error {
	// Read only .md files
	if d.IsDir() || filepath.Ext(d.Name()) != ".md" {
		return e
	}

	logger.Debug("Parsing article", "path", path)

	// Create article
	var obj Article
	obj.Fname, _ = strings.CutSuffix(d.Name(), ".md")

	// Open and parse metadata
	file, err := os.Open(path)
	check(err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			break
		}

		key, value, found := strings.Cut(line, " ")
		if !found {
			logger.Debug("Wrong metadata, skipping...")
			break
		}

		switch strings.ToLower(key) {
		case "title:":
			obj.Title = value
		case "date:":
			vdate, vtime, _ := strings.Cut(value, " ")
			ts, err := time.Parse("2006-01-02 15:04", value)
			check(err)

			obj.Date = vdate
			obj.Time = vtime
			obj.Timestamp = ts
		case "description:":
			obj.Description = value
		case "image:":
			obj.Image = value
		case "author:":
			obj.Author = value
		case "author_url:":
			obj.AuthorURL = value
		}
	}

	// Read and parse content
	var previewb bytes.Buffer
	var bodyb bytes.Buffer
	var cutMark = []byte("---CUT---")

	for scanner.Scan() {
		line := scanner.Bytes()

		if !obj.Cut {
			if bytes.Equal(line, cutMark) {
				obj.Cut = true
			} else {
				previewb.Write(line); bodyb.Write(line)
				previewb.WriteByte('\n'); bodyb.WriteByte('\n')
			}
		} else {
			bodyb.Write(line)
			bodyb.WriteByte('\n')
		}
	}

	obj.Preview = ToHTML(previewb.Bytes())

	// Handle article cut
	if obj.Cut {
		obj.Body = ToHTML(bodyb.Bytes())
	}

	// Parse category and create it if it doesn't exist
	// TODO: Handle no categories
	obj.Category = filepath.Base(filepath.Dir(path))

	// Parse URL
	obj.URL = filepath.Join(obj.Category, obj.Fname)
	if !cfg.PrettyURLs {
		obj.URL += ".html"
	}

	// Create share link
	obj.ShareURL = CreateShareURL(&obj)

	// Append new article
	articles = append(articles, &obj)

	// TODO: check_article()

	return e
}

func RenderPage(obj *Article) {
	logger.Debug("Rendering page", "page", obj.Title)

	ctx := PageContext{
		Title:       obj.Title + " // " + cfg.Title,
		Description: obj.Description,
		Image:       obj.Image,
		Cfg:         &cfg,
		Articles:    []*Article{obj},
		InArticle:   true,
	}

	outpath := filepath.Join(OutDir, obj.Category, obj.Fname+".html")
	Render(outpath, "main.html", ctx)
}

func RenderFronts(objs []*Article, category string) {
	totalPages := int(math.Ceil(float64(len(objs)) / float64(cfg.PageSize)))
	pages := make([]string, totalPages)

	/* Build page URLs */
	for page := range pages {
		var url string

		if page > 0 {
			url = strconv.Itoa(page)
		}

		if !cfg.PrettyURLs {
			url += ".html"
		}

		if category != "" {
			url = filepath.Join(category, url)
		}

		pages[page] = url
	}

	/* Generate pages */
	for page := range pages {
		start := page * cfg.PageSize
		end := start + cfg.PageSize

		var sliced []*Article

		if page == len(pages)-1 {
			sliced = objs[start:]
		} else {
			sliced = objs[start:end]
		}

		RenderFront(sliced, category, page, pages)
	}
}

func RenderFront(objs []*Article, category string, page int, pages []string) {
	logger.Debug("Processing page", "page", page, "category", category)

	var outpath string
	var title string
	var basename string

	if page > 0 {
		basename = strconv.Itoa(page) + ".html"
	} else {
		basename = "index.html"
	}

	if category != "" {
		title = category + " // " + cfg.Title
		outpath = filepath.Join(OutDir, category, basename)
	} else {
		title = cfg.Title
		outpath = filepath.Join(OutDir, basename)
	}

	ctx := PageContext{
		Title:       title,
		Description: cfg.Description,
		Cfg:         &cfg,
		Articles:    objs,
		Categories:  categoryTitles,
		InArticle:   false,
		CurrentPage: page,
		TotalPages:  len(pages),
		Pages:       pages,
	}

	Render(outpath, "main.html", ctx)
}

func Render(path string, name string, ctx any) {
	err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
	check(err)

	f, err := os.Create(path)
	check(err)

	defer f.Close()

	err = tmpl.ExecuteTemplate(f, name, ctx)
	check(err)
}

func ParseConfig() {
	f, err := os.Open("config.json")
	check(err)
	defer f.Close()

	decoder := json.NewDecoder(f)
	err = decoder.Decode(&cfg)
	check(err)
}

func main() {
	t := time.Now()
	categories = make(map[string][]*Article)

	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	logger = slog.New(slog.NewTextHandler(os.Stdout, opts))

	logger.Info("nikki " + Version)
	ParseConfig()

	logger.Info("Initializing parser...")
	md = goldmark.New(
		goldmark.WithRendererOptions(html.WithUnsafe()),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithExtensions(
			extension.Footnote,
			&admonitions.Extender{},
			extension.GFM,
			gmhtml5.New(
				gmhtml5.WithoutFigures(),
				gmhtml5.WithoutLazyImages(),
			),
			highlighting.NewHighlighting(
				highlighting.WithStyle("dracula"),
				highlighting.WithGuessLanguage(true),
			),
		),
	)

	logger.Info("Parsing articles in '" + PagesDir + "'...")
	filepath.WalkDir(PagesDir, ParseArticle)

	logger.Info("Arranging articles and categories...")
	ArrangeArticles()

	logger.Info("Reading templates...")
	tmpl = template.Must(template.ParseGlob("templates/*"))

	logger.Info("Rendering articles...")
	for _, obj := range articles {
		RenderPage(obj)
	}

	logger.Info("Sorting categories...")
	SortCategories()

	logger.Info("Rendering category pages...")
	for category, objs := range categories {
		logger.Debug("Rendering category", category, "...")
		RenderFronts(objs, category)
	}

	logger.Info("Rendering front page...")
	RenderFronts(articles, "")

	logger.Info("Done.", "time", time.Now().Sub(t))
}
