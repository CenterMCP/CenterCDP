package handlers

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rvarun11/sqlite-mcp/internal/cdp"
)

const tableResourceURI = "ui://centercdp/table/v1.html"
const tableMIMEType = "text/html;profile=mcp-app"

type tableColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type tablePagination struct {
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
	Total      int            `json:"total"`
	TotalPages int            `json:"total_pages"`
	Tool       string         `json:"tool,omitempty"`
	Arguments  map[string]any `json:"arguments,omitempty"`
}

type tableView struct {
	View       string          `json:"view"`
	Title      string          `json:"title"`
	Entity     string          `json:"entity"`
	Columns    []tableColumn   `json:"columns"`
	Rows       any             `json:"rows"`
	Pagination tablePagination `json:"pagination"`
}

func tableToolMeta() *mcp.Meta {
	return mcp.NewMetaFromMap(map[string]any{
		"ui":                             map[string]any{"resourceUri": tableResourceURI},
		"openai/outputTemplate":          tableResourceURI,
		"openai/toolInvocation/invoking": "Loading table…",
		"openai/toolInvocation/invoked":  "Table ready.",
	})
}

func registerTableResource(mcpServer *server.MCPServer) {
	resource := mcp.NewResource(
		tableResourceURI,
		"centercdp-table",
		mcp.WithResourceTitle("CenterCDP table"),
		mcp.WithResourceDescription("Paginated table for CenterCDP collection results."),
		mcp.WithMIMEType(tableMIMEType),
	)
	resource.Meta = mcp.NewMetaFromMap(map[string]any{
		"ui": map[string]any{"prefersBorder": true},
	})
	mcpServer.AddResource(resource, func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return []mcp.ResourceContents{mcp.TextResourceContents{
			URI:      tableResourceURI,
			MIMEType: tableMIMEType,
			Text:     tableHTML,
			Meta: map[string]any{
				"ui": map[string]any{"prefersBorder": true},
			},
		}}, nil
	})
}

func newPagedTable(title, entity, tool string, columns []tableColumn, page *cdp.PageResult, arguments map[string]any) tableView {
	if page == nil {
		page = &cdp.PageResult{}
	}
	return tableView{
		View:    "table",
		Title:   title,
		Entity:  entity,
		Columns: columns,
		Rows:    page.Rows,
		Pagination: tablePagination{
			Page: page.Page, PageSize: page.PageSize, Total: page.Total, TotalPages: page.TotalPages,
			Tool: tool, Arguments: arguments,
		},
	}
}

func tableResult(value any, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	raw, _ := json.MarshalIndent(value, "", "  ")
	return mcp.NewToolResultStructured(value, string(raw)), nil
}

func contactColumns(rows []map[string]any) []tableColumn {
	columns := []tableColumn{
		{Key: "contact_id", Label: "Contact ID"},
		{Key: "primary_identity", Label: "Primary identity"},
	}
	attributeKeys := map[string]bool{}
	for _, row := range rows {
		attributes, _ := row["attributes"].(map[string]any)
		for key := range attributes {
			attributeKeys[key] = true
		}
	}
	keys := make([]string, 0, len(attributeKeys))
	for key := range attributeKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		columns = append(columns, tableColumn{Key: "attributes." + key, Label: humanizeColumn(key)})
	}
	return append(columns,
		tableColumn{Key: "created_at", Label: "Created"},
		tableColumn{Key: "updated_at", Label: "Updated"},
	)
}

func humanizeColumn(value string) string {
	words := strings.Fields(strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(value))
	for i := range words {
		words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
	}
	return strings.Join(words, " ")
}

const tableHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
:root{color-scheme:light dark;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
*{box-sizing:border-box}body{margin:0;background:transparent;color:CanvasText}.card{min-width:0;padding:14px}
.heading{display:flex;align-items:baseline;justify-content:space-between;gap:12px;margin-bottom:10px}.heading h2{font-size:16px;line-height:1.3;margin:0}.count{font-size:12px;opacity:.65;white-space:nowrap}
.table-wrap{overflow:auto;border:1px solid color-mix(in srgb,CanvasText 14%,transparent);border-radius:10px}
table{border-collapse:collapse;width:100%;font-size:13px}th,td{padding:9px 11px;border-bottom:1px solid color-mix(in srgb,CanvasText 10%,transparent);text-align:left;vertical-align:top;white-space:nowrap}th{font-size:11px;letter-spacing:.025em;text-transform:uppercase;opacity:.7;background:color-mix(in srgb,Canvas 94%,CanvasText 6%);position:sticky;top:0}tbody tr:last-child td{border-bottom:0}td{max-width:320px;overflow:hidden;text-overflow:ellipsis}
.empty{padding:28px 14px;text-align:center;opacity:.65}.footer{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-top:10px;font-size:12px}.buttons{display:flex;gap:6px}button{appearance:none;border:1px solid color-mix(in srgb,CanvasText 18%,transparent);border-radius:7px;background:Canvas;color:CanvasText;padding:6px 10px;font:inherit;cursor:pointer}button:hover:not(:disabled){background:color-mix(in srgb,Canvas 92%,CanvasText 8%)}button:disabled{cursor:default;opacity:.4}.error{color:#b42318;padding:12px;border:1px solid currentColor;border-radius:8px}.loading{opacity:.55}
</style>
</head>
<body><main class="card" id="app" aria-live="polite"><div class="loading">Loading…</div></main>
<script>
const app=document.getElementById("app");
const pending=new Map();let nextID=1;let current=null;
function request(method,params){const id=nextID++;window.parent.postMessage({jsonrpc:"2.0",id,method,params},"*");return new Promise((resolve,reject)=>pending.set(id,{resolve,reject}));}
function valueAt(row,key){if(row==null)return null;if(Object.prototype.hasOwnProperty.call(row,key))return row[key];return key.split(".").reduce((value,part)=>value==null?null:value[part],row);}
function display(value){if(value===null||value===undefined||value==="")return "—";if(typeof value==="boolean")return value?"Yes":"No";if(typeof value==="object"){try{return JSON.stringify(value)}catch{return String(value)}}return String(value);}
function el(name,className,text){const node=document.createElement(name);if(className)node.className=className;if(text!==undefined)node.textContent=text;return node;}
function render(data){current=data;if(!data||data.view!=="table"){app.replaceChildren(el("div","error","This result cannot be displayed as a table."));return;}
 const rows=Array.isArray(data.rows)?data.rows:[];const p=data.pagination||{};let columns=Array.isArray(data.columns)?data.columns:[];
 if(!columns.length&&rows[0])columns=Object.keys(rows[0]).map(key=>({key,label:key}));
 const heading=el("div","heading");heading.append(el("h2","",data.title||"Results"),el("div","count",String(p.total??rows.length)+" total"));
 const wrap=el("div","table-wrap");if(!rows.length){wrap.append(el("div","empty","No results found."));}else{const table=el("table");const thead=el("thead");const header=el("tr");for(const col of columns)header.append(el("th","",col.label||col.key));thead.append(header);table.append(thead);const tbody=el("tbody");for(const row of rows){const tr=el("tr");for(const col of columns){const text=display(valueAt(row,col.key));const td=el("td","",text);td.title=text;tr.append(td);}tbody.append(tr);}table.append(tbody);wrap.append(table);}
 const page=p.page||1,totalPages=p.total_pages||0,start=rows.length?(page-1)*(p.page_size||rows.length)+1:0,end=rows.length?start+rows.length-1:0;
 const footer=el("div","footer");footer.append(el("div","",rows.length?"Rows "+start+"–"+end+" of "+String(p.total??rows.length):"0 rows"));const buttons=el("div","buttons");
 const previous=el("button","","Previous"),next=el("button","","Next");previous.disabled=page<=1||!p.tool;next.disabled=!p.tool||totalPages===0||page>=totalPages;previous.onclick=()=>changePage(page-1,previous);next.onclick=()=>changePage(page+1,next);buttons.append(previous,el("span","","Page "+page+" of "+Math.max(totalPages,1)),next);footer.append(buttons);app.replaceChildren(heading,wrap,footer);
}
async function changePage(page,button){if(!current?.pagination?.tool)return;button.disabled=true;app.classList.add("loading");try{const p=current.pagination;const args={...(p.arguments||{}),page,page_size:p.page_size};const result=await request("tools/call",{name:p.tool,arguments:args});if(result?.structuredContent)render(result.structuredContent);else throw new Error("The server returned no table data.");}catch(error){app.replaceChildren(el("div","error",error?.message||"Could not load this page."));}finally{app.classList.remove("loading");}}
window.addEventListener("message",event=>{if(event.source!==window.parent)return;const message=event.data;if(!message||message.jsonrpc!=="2.0")return;if(message.id!==undefined&&pending.has(message.id)){const waiter=pending.get(message.id);pending.delete(message.id);message.error?waiter.reject(message.error):waiter.resolve(message.result);return;}if(message.method==="ui/notifications/tool-result"&&message.params?.structuredContent)render(message.params.structuredContent);},{passive:true});
if(window.openai?.toolOutput)render(window.openai.toolOutput);
</script></body></html>`
