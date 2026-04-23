package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// ──────────────────────────────────────────────
// Reverse proxy for MLflow UI
// ──────────────────────────────────────────────

// newMLflowProxy creates a reverse proxy that forwards requests to
// the MLflow backend, rewriting paths and injecting portal UI elements.
func newMLflowProxy() *httputil.ReverseProxy {
	target, _ := url.Parse(mlflowURL)
	proxy := httputil.NewSingleHostReverseProxy(target)

	originalDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		originalDirector(r)
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/mlflow")
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
		r.Host = "localhost:5000"
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		if location := resp.Header.Get("Location"); location != "" {
			if strings.HasPrefix(location, "/") && !strings.HasPrefix(location, "/mlflow") {
				resp.Header.Set("Location", "/mlflow"+location)
			}
		}

		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, "text/html") {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			resp.Body.Close()

			html := string(body)
			html = strings.ReplaceAll(html, `href="/static-files/`, `href="/mlflow/static-files/`)
			html = strings.ReplaceAll(html, `src="/static-files/`, `src="/mlflow/static-files/`)
			html = strings.ReplaceAll(html, `"/ajax-api/`, `"/mlflow/ajax-api/`)

			// Inject portal buttons into MLflow's left sidebar
			html = strings.Replace(html, "</body>", portalSidebarHTML+"</body>", 1)

			resp.Body = io.NopCloser(bytes.NewReader([]byte(html)))
			resp.ContentLength = int64(len(html))
			resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(html)))
		}

		return nil
	}

	return proxy
}

// authProxyMiddleware wraps the reverse proxy with authentication,
// injecting Basic Auth credentials from the session cookies.
func authProxyMiddleware(proxy *httputil.ReverseProxy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := getClaimsFromRequest(r)
		if err != nil {
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		password := getPasswordFromToken(r)
		if password == "" {
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
			[]byte(claims.Username+":"+password),
		))

		proxy.ServeHTTP(w, r)
	})
}

// ──────────────────────────────────────────────
// Portal sidebar HTML injected into MLflow UI
// ──────────────────────────────────────────────

const portalSidebarHTML = `<style>
#portal-sidebar-inject{position:fixed;left:0;bottom:128px;width:60px;z-index:99999;display:flex;flex-direction:column;align-items:center;padding-bottom:0;font-family:'Inter',-apple-system,BlinkMacSystemFont,sans-serif;}
#portal-sidebar-inject .p-btn{display:flex;flex-direction:column;align-items:center;justify-content:center;gap:2px;width:100%;padding:8px 4px;cursor:pointer;border:none;background:transparent;text-decoration:none;transition:background .15s;border-radius:0;}
#portal-sidebar-inject .p-btn:hover{background:rgba(255,255,255,0.08);}
#portal-sidebar-inject .p-btn svg{opacity:0.7;}
#portal-sidebar-inject .p-btn:hover svg{opacity:1;}
#portal-sidebar-inject .p-label{font-size:10px;font-weight:500;line-height:1.2;white-space:nowrap;}
#portal-creds-overlay{display:none;position:fixed;inset:0;background:rgba(0,0,0,0.5);backdrop-filter:blur(4px);z-index:100000;align-items:center;justify-content:center;}
#portal-creds-overlay.active{display:flex;}
#portal-creds-card{background:#1a1a2e;border:1px solid rgba(255,255,255,0.1);border-radius:12px;padding:28px 32px;min-width:340px;box-shadow:0 20px 60px rgba(0,0,0,0.5);font-family:'Inter',-apple-system,sans-serif;animation:pcSlideUp .25s ease;}
@keyframes pcSlideUp{from{opacity:0;transform:translateY(12px);}to{opacity:1;transform:translateY(0);}}
#portal-creds-card h3{margin:0 0 20px;font-size:16px;font-weight:700;color:#fff;display:flex;align-items:center;gap:8px;}
#portal-creds-card .pc-row{margin-bottom:14px;}
#portal-creds-card .pc-lbl{font-size:11px;font-weight:600;color:#8892a4;text-transform:uppercase;letter-spacing:0.5px;margin-bottom:4px;}
#portal-creds-card .pc-val{display:flex;align-items:center;gap:8px;background:rgba(255,255,255,0.05);border:1px solid rgba(255,255,255,0.08);border-radius:6px;padding:10px 12px;font-size:14px;color:#e2e8f0;font-family:'SF Mono',Consolas,monospace;word-break:break-all;}
#portal-creds-card .pc-val .pc-copy{margin-left:auto;cursor:pointer;background:none;border:none;color:#64748b;padding:2px;border-radius:4px;display:flex;transition:color .15s;flex-shrink:0;}
#portal-creds-card .pc-val .pc-copy:hover{color:#00e5d0;}
#portal-creds-card .pc-close{width:100%;margin-top:8px;padding:10px;background:rgba(255,255,255,0.06);border:1px solid rgba(255,255,255,0.08);border-radius:8px;color:#94a3b8;font-size:13px;font-weight:600;cursor:pointer;transition:all .15s;}
#portal-creds-card .pc-close:hover{background:rgba(255,255,255,0.1);color:#fff;}
</style>
<div id="portal-sidebar-inject">
<a id="portal-admin-btn" href="/" class="p-btn" style="display:none;" title="Admin Panel">
<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#5bb8ff" stroke-width="2"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>
<span class="p-label" style="color:#5bb8ff;">Admin</span>
</a>
<button class="p-btn" onclick="document.getElementById('portal-creds-overlay').classList.add('active');portalLoadCreds();" title="Show Credentials">
<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#fbbf24" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
<span class="p-label" style="color:#fbbf24;">Creds</span>
</button>
<button class="p-btn" onclick="fetch('/api/logout',{method:'POST'}).then(()=>window.location.href='/')" title="Logout">
<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#f87171" stroke-width="2"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
<span class="p-label" style="color:#f87171;">Logout</span>
</button>
</div>
<div id="portal-creds-overlay" onclick="if(event.target===this)this.classList.remove('active');">
<div id="portal-creds-card">
<h3><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#00e5d0" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg> Your Credentials</h3>
<div class="pc-row"><div class="pc-lbl">Username</div><div class="pc-val"><span id="pc-user">—</span><button class="pc-copy" onclick="navigator.clipboard.writeText(document.getElementById('pc-user').textContent)" title="Copy"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg></button></div></div>
<div class="pc-row"><div class="pc-lbl">Password</div><div class="pc-val"><span id="pc-pass">—</span><button class="pc-copy" onclick="navigator.clipboard.writeText(document.getElementById('pc-pass').textContent)" title="Copy"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg></button></div></div>
<button class="pc-close" onclick="document.getElementById('portal-creds-overlay').classList.remove('active');">Close</button>
</div>
</div>
<script>
function portalLoadCreds(){fetch('/api/me/credentials').then(r=>r.json()).then(d=>{document.getElementById('pc-user').textContent=d.username||'—';document.getElementById('pc-pass').textContent=d.password||'—';}).catch(()=>{document.getElementById('pc-user').textContent='Error';document.getElementById('pc-pass').textContent='Error';});}
fetch('/api/me').then(r=>r.json()).then(d=>{if(d.is_admin){document.getElementById('portal-admin-btn').style.display='flex';}}).catch(()=>{});
</script>`
