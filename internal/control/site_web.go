package control

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed siteweb
var siteWebFiles embed.FS

// registerSiteWeb is independent of control state and the private admin entry.
// Only named pages and exact asset files are public; unknown paths stay 404.
func registerSiteWeb(mux *http.ServeMux) {
	for _, page := range []struct{ Path, Title, Description string }{
		{"/", "NodeLane Room · 让朋友回到同一个局域网", "为朋友之间的游戏联机而生。创建房间、邀请好友，在熟悉的游戏局域网中相聚。"},
		{"/product", "产品功能 · NodeLane Room", "了解 NodeLane Room 的游戏房间、好友邀请、连接状态和桌面体验。"},
		{"/download", "下载客户端 · NodeLane Room", "获取 NodeLane Room 客户端，了解 Windows 与 Linux 的安装和使用要求。"},
		{"/help", "帮助中心 · NodeLane Room", "从安装、创建房间到邀请朋友，了解 NodeLane Room 的使用方式和常见问题。"},
		{"/about", "关于我们 · NodeLane Room", "了解 NodeLane Room 的产品理念、公开项目和联系渠道。"},
		{"/privacy", "隐私说明 · NodeLane Room", "了解 NodeLane Room 在账号、设备、房间和网络诊断中使用的数据。"},
		{"/terms", "使用说明 · NodeLane Room", "了解 NodeLane Room 的产品用途、使用条件和支持范围。"},
		{"/en", "NodeLane Room · Bring your friends onto the same LAN", "Made for playing together. Create a room, invite your friends, and reconnect through the games you love."},
		{"/en/product", "Product · NodeLane Room", "Explore NodeLane Room game rooms, invitations, connection diagnostics, and the desktop experience."},
		{"/en/download", "Download · NodeLane Room", "Get the NodeLane Room desktop client and learn about installation requirements for Windows and Linux."},
		{"/en/help", "Help Center · NodeLane Room", "Learn how to install NodeLane Room, create a room, invite friends, and troubleshoot common connection issues."},
		{"/en/about", "About · NodeLane Room", "Learn about the ideas behind NodeLane Room, the project, and where to find support."},
		{"/en/privacy", "Privacy · NodeLane Room", "Learn how NodeLane Room uses account, device, room, and network diagnostic data."},
		{"/en/terms", "Usage Guide · NodeLane Room", "Understand NodeLane Room's intended use, requirements, and support scope."},
	} {
		data := struct {
			Path, Title, Description, Lang, Prefix, PagePath, SwitchPath string
			English                                                      bool
		}{Path: page.Path, Title: page.Title, Description: page.Description, Lang: "zh-CN", PagePath: page.Path, SwitchPath: "/en" + page.Path}
		name := page.Path[1:]
		if name == "" {
			name = "index"
			data.SwitchPath = "/en"
		}
		if page.Path == "/en" || strings.HasPrefix(page.Path, "/en/") {
			data.English, data.Lang, data.Prefix = true, "en", "/en"
			data.PagePath = strings.TrimPrefix(page.Path, "/en")
			if data.PagePath == "" {
				data.PagePath, name = "/", "en/index"
			}
			data.SwitchPath = data.PagePath
		}
		view := template.Must(template.ParseFS(siteWebFiles, "siteweb/layout.html", "siteweb/"+name+".html"))
		var content bytes.Buffer
		if err := view.ExecuteTemplate(&content, "layout", data); err != nil {
			panic(err) // Embedded templates must be valid before HTTP starts.
		}
		html := content.Bytes()
		pattern := "GET " + page.Path
		if page.Path == "/" {
			pattern += "{$}"
		}
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Language", data.Lang)
			if r.Method != http.MethodHead {
				_, _ = w.Write(html)
			}
		})
	}
	assets, err := fs.Sub(siteWebFiles, "siteweb/assets")
	if err != nil {
		panic(err)
	}
	entries, err := fs.ReadDir(assets, ".")
	if err != nil {
		panic(err)
	}
	const prefix = "/site-assets/"
	files := http.StripPrefix(prefix, http.FileServer(http.FS(assets)))
	for _, entry := range entries {
		if !entry.IsDir() {
			mux.Handle("GET "+prefix+entry.Name(), files)
		}
	}
}
