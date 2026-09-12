package control

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

var publicSitePaths = []string{
	"/", "/product", "/download", "/help", "/about", "/privacy", "/terms",
	"/en", "/en/product", "/en/download", "/en/help", "/en/about", "/en/privacy", "/en/terms",
}

func TestPublicSiteAcrossDeploymentStates(t *testing.T) {
	const entry = "/private-site-test-entry"
	ready := &Server{AdminPath: entry}
	configured := &Deployment{AdminPath: entry}
	configured.active.Store(&runningControl{server: ready, handler: ready.Handler()})
	for name, handler := range map[string]http.Handler{
		"unconfigured": (&Deployment{AdminPath: entry}).Handler(),
		"configured":   configured.Handler(),
		"server":       ready.Handler(),
		"no_admin":     (&Server{}).Handler(),
	} {
		t.Run(name, func(t *testing.T) {
			bodies := map[string]bool{}
			for _, path := range publicSitePaths {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
				if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
					t.Fatalf("%s: status %d, content type %q", path, w.Code, w.Header().Get("Content-Type"))
				}
				body := w.Body.String()
				if strings.Contains(body, entry) || strings.Contains(body, "/admin/") || strings.Contains(body, "/v2/admin") || strings.Contains(body, "./assets/app.js") {
					t.Fatalf("%s discloses the private application", path)
				}
				if w.Header().Get("Content-Security-Policy") == "" || strings.Contains(w.Header().Get("X-Robots-Tag"), "noindex") || w.Header().Get("Set-Cookie") != "" {
					t.Fatalf("%s has incorrect public page headers", path)
				}
				if !strings.Contains(body, "NodeLane Room") || !strings.Contains(body, "<main") || bodies[body] {
					t.Fatalf("%s does not render its own complete page", path)
				}
				bodies[body] = true
				w = httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest(http.MethodHead, path, nil))
				if w.Code != http.StatusOK || w.Body.Len() != 0 {
					t.Fatalf("HEAD %s: status %d, body length %d", path, w.Code, w.Body.Len())
				}
			}
			for _, path := range []string{"/admin", "/admin/", "/assets/app.js", "/site-assets/", "/site-assets/layout.html", "/site-assets/missing.png", "/site-assets/%2e%2e/layout.html", "/siteweb/layout.html", "/layout.html", "/index.html", "/unknown", "/product/unknown", "/en/admin", "/en/unknown", "/en/product/unknown", "/en/index.html", "/siteweb/en/index.html"} {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
				if w.Code != http.StatusNotFound || w.Header().Get("Location") != "" || strings.Contains(w.Body.String(), entry) {
					t.Fatalf("unexpected public resource at %s: %d", path, w.Code)
				}
			}
		})
	}
}

func TestPublicSiteLocalLinksAndAssets(t *testing.T) {
	handler := (&Server{AdminPath: "/private-site-test-entry"}).Handler()
	links := regexp.MustCompile(`(?:href|src)="(/[^"#]*)`)
	for _, path := range publicSitePaths {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		for _, match := range links.FindAllStringSubmatch(w.Body.String(), -1) {
			if strings.HasPrefix(match[1], "//") {
				continue
			}
			linked := httptest.NewRecorder()
			handler.ServeHTTP(linked, httptest.NewRequest(http.MethodGet, match[1], nil))
			if linked.Code != http.StatusOK {
				t.Errorf("%s links to unavailable %s: %d", path, match[1], linked.Code)
			}
		}
	}
	entries, err := fs.ReadDir(siteWebFiles, "siteweb/assets")
	must(t, err)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := "/site-assets/" + entry.Name()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK || w.Body.Len() == 0 || w.Header().Get("Content-Type") == "" {
			t.Errorf("asset %s: status %d, body length %d", path, w.Code, w.Body.Len())
		}
		if strings.HasPrefix(entry.Name(), "app-") {
			config, format, err := image.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
			if err != nil || config.Width != 1280 || config.Height != 800 || w.Header().Get("Content-Type") != "image/"+format {
				t.Errorf("screenshot %s must be 1280x800 with the correct media type: %v, %s, %+v", path, err, format, config)
			}
		}
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST asset %s: status %d", path, w.Code)
		}
	}
}

func TestPublicSiteLanguageSwitchKeepsPage(t *testing.T) {
	handler := (&Server{}).Handler()
	metadata := regexp.MustCompile(`<title>([^<]+)</title>|<meta name="description" content="([^"]+)"`)
	chinese := regexp.MustCompile(`\p{Han}`)
	for _, page := range []struct{ Chinese, English string }{
		{"/", "/en"}, {"/product", "/en/product"}, {"/download", "/en/download"}, {"/help", "/en/help"},
		{"/about", "/en/about"}, {"/privacy", "/en/privacy"}, {"/terms", "/en/terms"},
	} {
		for _, language := range []struct{ Path, Switch, Lang string }{
			{page.Chinese, page.English, "zh-CN"}, {page.English, page.Chinese, "en"},
		} {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, language.Path, nil))
			body := w.Body.String()
			if w.Header().Get("Content-Language") != language.Lang || !strings.Contains(body, `<html lang="`+language.Lang+`"`) {
				t.Errorf("%s has incorrect document language", language.Path)
			}
			alternates := map[string]string{}
			switchFound := false
			tokens := html.NewTokenizer(strings.NewReader(body))
			for tokenType := tokens.Next(); tokenType != html.ErrorToken; tokenType = tokens.Next() {
				if tokenType != html.StartTagToken && tokenType != html.SelfClosingTagToken {
					continue
				}
				token := tokens.Token()
				attrs := map[string]string{}
				for _, attr := range token.Attr {
					attrs[attr.Key] = attr.Val
				}
				if token.Data == "link" && attrs["rel"] == "alternate" {
					alternates[attrs["hreflang"]] = attrs["href"]
				}
				if token.Data != "a" {
					continue
				}
				href := attrs["href"]
				if href == language.Switch && ((language.Lang == "en" && attrs["lang"] == "zh-CN") || (language.Lang == "zh-CN" && attrs["lang"] == "en")) {
					switchFound = true
					continue
				}
				if !strings.HasPrefix(href, "/") || strings.HasPrefix(href, "//") || strings.HasPrefix(href, "/site-assets/") {
					continue
				}
				englishLink := href == "/en" || strings.HasPrefix(href, "/en/")
				if englishLink != (language.Lang == "en") {
					t.Errorf("%s links to another language outside its language switch: %s", language.Path, href)
				}
			}
			if !switchFound || alternates["zh-CN"] != page.Chinese || alternates["en"] != page.English || alternates["x-default"] != page.Chinese {
				t.Errorf("%s is missing its reciprocal page switch or language alternates", language.Path)
			}
			if language.Lang == "en" {
				for _, match := range metadata.FindAllStringSubmatch(body, -1) {
					if chinese.MatchString(match[1] + match[2]) {
						t.Errorf("%s has untranslated page metadata", language.Path)
					}
				}
			}
		}
	}
}
