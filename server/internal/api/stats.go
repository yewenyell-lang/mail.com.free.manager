package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"

	"mailcom/manager/internal/stats"

	"github.com/gin-gonic/gin"
)

const statsPageHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>使用统计 · mail.com 工作台</title>
<style>
  :root { --bg:#12110e; --panel:#1b1a16; --panel2:#26241e; --fg:#f4efe4; --mute:#c9c2b4; --gold:#e8a317; --line:rgba(244,239,228,.1); }
  * { box-sizing:border-box; }
  body { margin:0; padding:24px; background:var(--bg); color:var(--fg); font:14px/1.6 "Segoe UI","Microsoft YaHei UI",sans-serif; }
  h1 { font-size:18px; letter-spacing:.08em; margin:0 0 4px; }
  .sub { color:var(--mute); font-size:12px; margin-bottom:20px; }
  .cards { display:grid; grid-template-columns:repeat(auto-fill,minmax(150px,1fr)); gap:12px; margin-bottom:24px; }
  .card { background:var(--panel); border:1px solid var(--line); border-radius:8px; padding:14px 16px; }
  .card .v { font-size:22px; font-weight:600; color:var(--gold); }
  .card .k { font-size:12px; color:var(--mute); margin-top:2px; }
  .panel { background:var(--panel); border:1px solid var(--line); border-radius:8px; padding:16px; margin-bottom:24px; }
  .panel h2 { font-size:13px; margin:0 0 12px; color:var(--mute); letter-spacing:.06em; }
  table { width:100%; border-collapse:collapse; font-size:13px; }
  th,td { text-align:left; padding:6px 8px; border-bottom:1px solid var(--line); }
  th { color:var(--mute); font-weight:500; }
  td.num,th.num { text-align:right; font-variant-numeric:tabular-nums; }
  .barwrap { display:flex; align-items:flex-end; gap:6px; height:120px; }
  .barcol { flex:1; display:flex; flex-direction:column; justify-content:flex-end; gap:2px; height:100%; }
  .barv { background:var(--gold); border-radius:3px 3px 0 0; min-height:2px; }
  .baru { background:#5a5648; border-radius:3px 3px 0 0; min-height:2px; }
  .barx { font-size:10px; color:var(--mute); text-align:center; margin-top:6px; white-space:nowrap; overflow:hidden; }
  .legend { font-size:12px; color:var(--mute); margin-top:8px; }
  .dot { display:inline-block; width:8px; height:8px; border-radius:2px; margin-right:4px; }
</style>
</head>
<body>
<h1>mail.com 工作台 · 使用统计</h1>
<div class="sub" id="meta">加载中…</div>
<div class="cards" id="cards"></div>
<div class="panel">
  <h2>近 30 天访问</h2>
  <div class="barwrap" id="bars"></div>
  <div class="barwrap" id="barlabels" style="height:auto;display:flex;"></div>
  <div class="legend"><span class="dot" style="background:var(--gold)"></span>页面浏览 <span class="dot" style="background:#5a5648;margin-left:12px"></span>独立访客</div>
</div>
<div class="panel">
  <h2>接口调用排行</h2>
  <table><thead><tr><th>接口</th><th class="num">次数</th></tr></thead><tbody id="eps"></tbody></table>
</div>
<script>
async function main(){
  const r = await fetch('/api/admin/stats');
  if(!r.ok){ document.getElementById('meta').textContent='加载失败: '+r.status; return; }
  const d = await r.json();
  const t = d.totals||{};
  document.getElementById('meta').textContent = '运行自 ' + new Date(d.startedAt).toLocaleString() + ' · 生成于 ' + new Date(d.now).toLocaleString();
  const cards = [
    ['页面浏览', t.views], ['独立访客(今日)', todayUniques(d)],
    ['登录成功', t.loginsOk], ['登录失败', t.loginsFail],
    ['收信邮件数', t.mailsListed], ['读信次数', t.mailsOpened],
    ['发信', t.sends], ['回复', t.replies], ['转发', t.forwards], ['接口总调用', t.apiTotal],
  ];
  document.getElementById('cards').innerHTML = cards.map(c =>
    '<div class="card"><div class="v">'+fmt(c[1])+'</div><div class="k">'+c[0]+'</div></div>').join('');
  const days = (d.days||[]).slice(-30);
  const max = Math.max(1, ...days.map(x=>x.views));
  document.getElementById('bars').innerHTML = days.map(x =>
    '<div class="barcol" title="'+x.date+' 浏览 '+x.views+' / 访客 '+x.uniques+'">'+
    '<div class="barv" style="height:'+Math.round(x.views/max*100)+'%"></div>'+
    '<div class="baru" style="height:'+Math.round(x.uniques/max*100)+'%"></div></div>').join('');
  document.getElementById('barlabels').innerHTML = days.map(x =>
    '<div class="barx">'+x.date.slice(5)+'</div>').join('');
  document.getElementById('eps').innerHTML = (d.endpoints||[]).map(e =>
    '<tr><td>'+e.path+'</td><td class="num">'+fmt(e.count)+'</td></tr>').join('');
}
function todayUniques(d){ const days=d.days||[]; return days.length?days[days.length-1].uniques:0; }
function fmt(n){ return (n==null?0:n).toLocaleString(); }
main();
</script>
</body>
</html>`

func randomToken() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func (h *handlers) adminAuth() gin.HandlerFunc {
	password := os.Getenv("STATS_PASSWORD")
	return func(c *gin.Context) {
		if password == "" {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		user, pass, hasAuth := c.Request.BasicAuth()
		if !hasAuth || user != "admin" || !stats.SafeEqual(pass, password) {
			c.Header("WWW-Authenticate", `Basic realm="mailcom-stats"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

func (h *handlers) adminStatsPage(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(statsPageHTML))
}

func (h *handlers) adminStatsJSON(c *gin.Context) {
	c.JSON(http.StatusOK, h.stats.Snapshot())
}

func visitorToken(c *gin.Context) string {
	if cookie, err := c.Cookie("sd_uid"); err == nil && cookie != "" {
		return cookie
	}
	token := randomToken()
	c.SetCookie("sd_uid", token, 3600*24*365, "/", "", false, true)
	return token
}
