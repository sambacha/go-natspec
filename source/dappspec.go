// **Dappspec** is a Go port of [Docco](http://jashkenas.github.com/docco/): the
// original quick-and-dirty, hundred-line-long, literate-programming-style
// documentation generator. It produces HTML that displays your comments
// alongside your code. Comments are passed through
// [Markdown](http://daringfireball.net/projects/markdown/syntax), and code is
// passed through [Pygments](http://pygments.org/) syntax highlighting.  This
// page is the result of running Dappspec against its own source file.
//
// If you install Dappspec, you can run it from the command-line:
//
// dappspec *.go
//
// ...will generate an HTML documentation page for each of the named source
// files, with a menu linking to the other pages, saving it into a `docs`
// folder.
//
// The [source for Dappspec](http://github.com/sambacha/dappspec) is available on
// GitHub, and released under the MIT license.
//
// To install Dappspec, first make sure you have [Pygments](http://pygments.org/)
// Then, with the go tool:
//
//	go get github.com/sambacha/dappspec
package main

import (
	"bytes"
	"container/list"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template"

	"github.com/russross/blackfriday"
)

// ## Types
// Due to Go's statically typed nature, what is passed around in object
// literals in Docco, requires various structures

// A `Section` captures a piece of documentation and code
// Every time interleaving code is found between two comments
// a new `Section` is created.
type Section struct {
	docsText      []byte
	codeText      []byte
	firstCodeLine string
	DocsHTML      []byte
	CodeHTML      []byte
}

// a `TemplateSection` is a section that can be passed
// to Go's templating system, which expects strings.
type TemplateSection struct {
	DocsHTML   string
	CodeHTML   string
	SectionTag string
}

// a `Language` describes a programming language
type Language struct {
	// the `Pygments` name of the language
	name string
	// The comment delimiter
	symbol string
	// The regular expression to match the comment delimiter
	commentMatcher *regexp.Regexp
	// Used as a placeholder so we can parse back Pygments output
	// and put the sections together
	dividerText string
	// The HTML equivalent
	dividerHTML *regexp.Regexp
}

// a `TemplateData` is per-file
type TemplateData struct {
	// Title of the HTML output
	Title string
	// The Sections making up this file
	Sections []*TemplateSection
	// A full list of source files so that a table-of-contents can
	// be generated
	Sources []string
	// Only generate the TOC is there is more than one file
	// Go's templating system does not allow expressions in the
	// template, so calculate it outside
	Multiple bool
}

// a map of all the languages we know
var languages map[string]*Language

// paths of all the source files, sorted
var sources []string

// absolute path to get resources
var packageLocation string

// Wrap the code in these
const highlightStart = "<div class=\"highlight\"><pre>"
const highlightEnd = "</pre></div>"

// ## Main documentation generation functions

// Generate the documentation for a single source file
// by splitting it into sections, highlighting each section
// and putting it together.
// The WaitGroup is used to signal we are done, so that the main
// goroutine waits for all the sub goroutines
func generateDocumentation(source string, wg *sync.WaitGroup) {
	code, err := ioutil.ReadFile(source)
	if err != nil {
		log.Panic(err)
	}
	sections := parse(source, code)
	highlight(source, sections)
	generateHTML(source, sections)
	wg.Done()
}

// Parse splits code into `Section`s
func parse(source string, code []byte) *list.List {
	lines := bytes.Split(code, []byte("\n"))
	sections := new(list.List)
	sections.Init()
	language := getLanguage(source)

	var hasCode bool
	var codeText = new(bytes.Buffer)
	var docsText = new(bytes.Buffer)

	// save a new section
	save := func(docs, code []byte, firstCodeLine string) {
		// deep copy the slices since slices always refer to the same storage
		// by default
		docsCopy, codeCopy := make([]byte, len(docs)), make([]byte, len(code))
		copy(docsCopy, docs)
		copy(codeCopy, code)

		sections.PushBack(&Section{docsCopy, codeCopy, firstCodeLine, nil, nil})
	}

	var firstCodeLine string
	for _, line := range lines {
		// if the line is a comment
		if language.commentMatcher.Match(line) {
			// but there was previous code
			if hasCode {
				// we need to save the existing documentation and text
				// as a section and start a new section since code blocks
				// have to be delimited before being sent to Pygments
				save(docsText.Bytes(), codeText.Bytes(), firstCodeLine)
				hasCode = false
				codeText.Reset()
				docsText.Reset()
			}
			docsText.Write(language.commentMatcher.ReplaceAll(line, nil))
			docsText.WriteString("\n")
		} else {
			if !hasCode {
				firstCodeLine = string(line)
			}
			hasCode = true
			codeText.Write(line)
			codeText.WriteString("\n")
		}
	}
	// save any remaining parts of the source file
	save(docsText.Bytes(), codeText.Bytes(), firstCodeLine)
	return sections
}

// `highlight` pipes the source to Pygments, section by section
// delimited by dividerText, then reads back the highlighted output,
// searches for the delimiters and extracts the HTML version of the code
// and documentation for each `Section`
func highlight(source string, sections *list.List) {
	language := getLanguage(source)
	pygments := exec.Command("pygmentize", "-l", language.name, "-f", "html", "-O", "encoding=utf-8")
	pygmentsInput, _ := pygments.StdinPipe()
	pygmentsOutput, _ := pygments.StdoutPipe()
	// start the process before we start piping data to it
	// otherwise the pipe may block
	pygments.Start()
	for e := sections.Front(); e != nil; e = e.Next() {
		pygmentsInput.Write(e.Value.(*Section).codeText)
		if e.Next() != nil {
			io.WriteString(pygmentsInput, language.dividerText)
		}
	}
	pygmentsInput.Close()

	buf := new(bytes.Buffer)
	io.Copy(buf, pygmentsOutput)

	output := buf.Bytes()
	output = bytes.Replace(output, []byte(highlightStart), nil, -1)
	output = bytes.Replace(output, []byte(highlightEnd), nil, -1)

	for e := sections.Front(); e != nil; e = e.Next() {
		index := language.dividerHTML.FindIndex(output)
		if index == nil {
			index = []int{len(output), len(output)}
		}

		fragment := output[0:index[0]]
		output = output[index[1]:]
		e.Value.(*Section).CodeHTML = bytes.Join([][]byte{[]byte(highlightStart), []byte(highlightEnd)}, fragment)
		e.Value.(*Section).DocsHTML = blackfriday.MarkdownCommon(e.Value.(*Section).docsText)
	}
}

// compute the output location (in `docs/`) for the file
func destination(source string) string {
	base := filepath.Base(source)
	return "docs/" + base[0:strings.LastIndex(base, filepath.Ext(base))] + ".html"
}

func destinationTOC(source string) string {
	title := filepath.Base(source)
	title = strings.TrimSuffix(title, filepath.Ext(source))
	title = strings.TrimPrefix(title, "docs_")
	return title + ".html"
}

func titleTOC(source string) string {
	title := filepath.Base(source)
	title = strings.TrimSuffix(title, filepath.Ext(source))
	title = strings.TrimPrefix(title, "docs_")
	return title
}

func getSectionTag(index int, firstCodeLine string) string {
	if !strings.HasPrefix(firstCodeLine, "notice") &&
		!strings.HasPrefix(firstCodeLine, "dev") &&
		!strings.HasPrefix(firstCodeLine, "params") &&
		!strings.HasPrefix(firstCodeLine, "return") {
		// not a declaration
		return fmt.Sprintf("%d", index)
	}
	// a type or variable declaration
	parts := strings.Split(firstCodeLine, " ")
	return strings.TrimSpace(parts[1])
}

func getFieldOrType(firstCodeLine string) string {
	if !strings.HasPrefix(firstCodeLine, "notice") &&
		!strings.HasPrefix(firstCodeLine, "dev") &&
		!strings.HasPrefix(firstCodeLine, "params") &&
		!strings.HasPrefix(firstCodeLine, "return") {
		// not a declaration, field maybe?
		parts := strings.Split(firstCodeLine, " ")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
		return ""
	}
	// a type or variable declaration
	parts := strings.Split(firstCodeLine, " ")
	if len(parts) > 0 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func highlightRefs(text []byte, ref string) []byte {
	if len(ref) == 0 {
		return text
	}
	// weird shit as \A and \z matching doesn't work?
	ref = regexp.QuoteMeta(ref)
	rx := regexp.MustCompile(fmt.Sprintf(`([\s]+)(%s)`, ref))
	text = rx.ReplaceAll(text, []byte("$1<strong>$2</strong>"))
	rx = regexp.MustCompile(fmt.Sprintf(`(%s)([\s]+)`, ref))
	text = rx.ReplaceAll(text, []byte("<strong>$1</strong>$2"))
	rx = regexp.MustCompile(fmt.Sprintf(`([\s]+)(%s)([\s]+)`, ref))
	text = rx.ReplaceAll(text, []byte("$1<strong>$2</strong>$3"))
	return text
}

var (
	referenceRx  = regexp.MustCompile(`@@([\w]+)`)
	referenceTpl = []byte(`<a href="#section-$1" title="Jump to $1">$1</a>`)
)

// render the final HTML
func generateHTML(source string, sections *list.List) {
	title := filepath.Base(source)
	title = strings.TrimSuffix(title, filepath.Ext(source))
	title = strings.TrimPrefix(title, "docs_")

	dest := destination(source)
	// convert every `Section` into corresponding `TemplateSection`
	sectionsArray := make([]*TemplateSection, 0, sections.Len())
	for e, i := sections.Front(), 0; e != nil; e, i = e.Next(), i+1 {
		var sec = e.Value.(*Section)
		sectionTag := getSectionTag(i+1, sec.firstCodeLine)

		sec.DocsHTML = referenceRx.ReplaceAll(sec.DocsHTML, referenceTpl)
		sec.DocsHTML = highlightRefs(sec.DocsHTML, getFieldOrType(sec.firstCodeLine))
		section := &TemplateSection{
			DocsHTML:   string(sec.DocsHTML),
			SectionTag: sectionTag,
		}
		if !bytes.HasPrefix(sec.codeText, []byte("pragma")) &&
			!bytes.HasPrefix(sec.codeText, []byte("import")) {
			section.CodeHTML = string(sec.CodeHTML)
		}
		sectionsArray = append(sectionsArray, section)
	}
	// run through the Go template
	html := dappspecTemplate(TemplateData{title, sectionsArray, sources, len(sources) > 1})
	log.Println("dappspec: ", source, " -> ", dest)
	ioutil.WriteFile(dest, html, 0644)
}

func dappspecTemplate(data TemplateData) []byte {
	// this hack is required because `ParseFiles` doesn't
	// seem to work properly, always complaining about empty templates
	t, err := template.New("dappspec").Funcs(
		// introduce the two functions that the template needs
		template.FuncMap{
			"title":       titleTOC,
			"destination": destinationTOC,
		}).Parse(HTML)
	if err != nil {
		panic(err)
	}
	buf := new(bytes.Buffer)
	err = t.Execute(buf, data)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// get a `Language` given a path
func getLanguage(source string) *Language {
	return languages[filepath.Ext(source)]
}

// make sure `docs/` exists
func ensureDirectory(name string) {
	os.MkdirAll(name, 0755)
}

func setupLanguages() {
	languages = make(map[string]*Language)
	// you should add more languages here
	// only the first two fields should change, the rest should
	// be `nil, "", nil`
	languages[".sol"] = &Language{"solidity", "///", nil, "", nil}
}

func setup() {
	setupLanguages()

	// create the regular expressions based on the language comment symbol
	for _, lang := range languages {
		lang.commentMatcher, _ = regexp.Compile("^\\s*" + lang.symbol + "\\s?")
		lang.dividerText = "\n" + lang.symbol + "DIVIDER\n"
		lang.dividerHTML, _ = regexp.Compile("\\n*<span class=\"c1?\">" + lang.symbol + "DIVIDER<\\/span>\\n*")
	}
}

// let's Go!
func main() {
	setup()

	flag.Parse()
	sources = flag.Args()
	sort.Strings(sources)

	if flag.NArg() <= 0 {
		return
	}

	ensureDirectory("docs")
	ioutil.WriteFile("docs/dappspec.css", bytes.NewBufferString(Css).Bytes(), 0755)

	wg := new(sync.WaitGroup)
	wg.Add(flag.NArg())
	for _, arg := range flag.Args() {
		go generateDocumentation(arg, wg)
	}
	wg.Wait()
}
