package comrad

import (
	"io"
	"net/http"
)

func (m *Manager) handleOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, openAPISpec())
}

func (m *Manager) handleOpenAPIDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.WriteString(w, openAPIDocsHTML())
}

func openAPISpec() map[string]any {
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "COMRAD API",
			"version":     Version,
			"description": "COMRAD Manager client, admin, and Worker control API.",
		},
		"servers": []map[string]string{{"url": "/", "description": "current Manager"}},
		"tags": []map[string]string{
			{"name": "Health", "description": "Liveness, readiness, and metrics"},
			{"name": "Client", "description": "OpenAI-compatible client API"},
			{"name": "Admin", "description": "Manager administration API"},
			{"name": "Worker", "description": "Outbound Worker control and artifact delivery"},
		},
		"paths":      openAPIPaths(),
		"components": openAPIComponents(),
	}
}

func openAPIPaths() map[string]any {
	return map[string]any{
		"/health":                           getOperation("Health", "Manager liveness", nil, "PlainOK"),
		"/ready":                            getOperation("Health", "Manager readiness", nil, "PlainOK"),
		"/metrics":                          getOperation("Health", "Prometheus metrics", nil, "PrometheusText"),
		"/v1/models":                        getOperation("Client", "List available chat models", clientSecurity(), "ModelList"),
		"/v1/chat/completions":              postOperation("Client", "Create a chat completion task", clientSecurity(), "ChatCompletionRequest", "ChatCompletionChunk"),
		"/v1/jobs/{taskId}":                 getOperation("Client", "Get a submitted task", clientSecurity(), "Task"),
		"/v1/jobs/{taskId}/cancel":          postOperation("Client", "Cancel a submitted task", clientSecurity(), "", "StatusResponse"),
		"/api/admin/state":                  getOperation("Admin", "Get dashboard state", adminSecurity(), "StateResponse"),
		"/api/admin/state/ws-ticket":        postOperation("Admin", "Issue a short-lived dashboard WebSocket ticket", adminSecurity(), "", "AdminStateWSTicketResponse"),
		"/api/admin/state/ws":               getOperation("Admin", "Dashboard state WebSocket stream using a short-lived ticket", nil, "WebSocketUpgrade"),
		"/api/admin/nodes":                  adminReadWrite("List or update nodes", "UpdateNodeRequest", "NodeList"),
		"/api/admin/slots":                  getOperation("Admin", "List slots", adminSecurity(), "SlotList"),
		"/api/admin/artifacts":              adminReadWrite("List or register artifacts", "CreateArtifactRequest", "ArtifactList"),
		"/api/admin/artifacts/upload":       uploadOperation(),
		"/api/admin/artifacts/{artifactId}": deleteOperation("Admin", "Delete an unused artifact", adminSecurity(), "StatusResponse"),
		"/api/admin/nodes/{nodeId}/artifacts/{artifactId}": nodeArtifactEvictionOperation(),
		"/api/admin/profiles":                              adminProfilesOperation(),
		"/api/admin/profiles/compute-cost":                 postOperation("Admin", "Set profile compute cost", adminSecurity(), "SetProfileComputeCostRequest", "WorkloadProfile"),
		"/api/admin/policies":                              adminReadWrite("List or upsert placement policies", "UpsertPolicyRequest", "PolicyList"),
		"/api/admin/users":                                 adminUsersOperation(),
		"/api/admin/users/adjust-balance":                  postOperation("Admin", "Append an admin compute adjustment", adminSecurity(), "AdminBalanceAdjustmentRequest", "ComputeLedgerEntry"),
		"/api/admin/api-keys":                              adminReadWrite("List or issue client API keys", "IssueAPIKeyRequest", "APIKeyList"),
		"/api/admin/api-keys/lookup":                       postOperation("Admin", "Find the API client for a raw key", adminSecurity(), "APIKeyLookupRequest", "APIKeyLookupResponse"),
		"/api/admin/api-keys/revoke":                       postOperation("Admin", "Revoke a client API key", adminSecurity(), "RevokeAPIKeyRequest", "StatusResponse"),
		"/api/admin/placement":                             getOperation("Admin", "Preview placement state", adminSecurity(), "PlacementState"),
		"/api/admin/placement/explain":                     getOperation("Admin", "Explain placement dry run", adminSecurity(), "PlacementExplainResponse"),
		"/api/admin/placement/apply":                       postOperation("Admin", "Apply placement", adminSecurity(), "", "StatusResponse"),
		"/api/admin/tasks":                                 getOperation("Admin", "List paginated tasks", adminSecurity(), "TaskListResponse"),
		"/api/admin/attempts":                              getOperation("Admin", "List attempts", adminSecurity(), "AttemptList"),
		"/api/admin/reports":                               getOperation("Admin", "List compute reports", adminSecurity(), "ReportList"),
		"/api/admin/quarantine/unban":                      postOperation("Admin", "Manually unban a node or slot", adminSecurity(), "UnbanRequest", "StatusResponse"),
		"/api/admin/updates":                               getOperation("Admin", "List worker updates", adminSecurity(), "UpdateList"),
		"/api/admin/updates/workers/apply":                 postOperation("Admin", "Create and dispatch a Worker update", adminSecurity(), "ApplyWorkerUpdateRequest", "UpdateRecord"),
		"/api/admin/metrics":                               getOperation("Admin", "Authenticated Prometheus metrics", adminSecurity(), "PrometheusText"),
		"/api/admin/worker-join":                           getOperation("Admin", "Get a Worker install command", adminSecurity(), "WorkerJoinResponse"),
		"/api/admin/config.yaml":                           getOperation("Admin", "Get sanitized runtime YAML config", adminSecurity(), "YAMLDocument"),
		"/api/admin/openapi.json":                          getOperation("Admin", "Get this OpenAPI document", adminSecurity(), "OpenAPIDocument"),
		"/api/admin/docs":                                  getOperation("Admin", "Open the built-in API reference", adminSecurity(), "HTMLDocument"),
		"/api/worker/ws":                                   getOperation("Worker", "Worker WebSocket control stream", workerSecurity(), "WebSocketUpgrade"),
		"/api/worker/artifacts/{artifactId}":               getOperation("Worker", "Download an assigned artifact", workerSecurity(), "BinaryArtifact"),
	}
}

func adminReadWrite(summary, requestSchema, responseSchema string) map[string]any {
	return map[string]any{
		"get":  operation("get", "Admin", summary, adminSecurity(), "", responseSchema),
		"post": operation("post", "Admin", summary, adminSecurity(), requestSchema, responseSchema),
	}
}

func adminProfilesOperation() map[string]any {
	del := operation("delete", "Admin", "Delete a workload profile", adminSecurity(), "", "StatusResponse")
	del["parameters"] = []map[string]any{parameter("profileId", "query", true)}
	return map[string]any{
		"get":    operation("get", "Admin", "List or upsert workload profiles", adminSecurity(), "", "ProfileList"),
		"post":   operation("post", "Admin", "List or upsert workload profiles", adminSecurity(), "CreateProfileRequest", "ProfileList"),
		"delete": del,
	}
}

func nodeArtifactEvictionOperation() map[string]any {
	del := operation("delete", "Admin", "Evict a cached artifact from a selected Worker", adminSecurity(), "", "StatusResponse")
	del["parameters"] = []map[string]any{
		parameter("nodeId", "path", true),
		parameter("artifactId", "path", true),
	}
	del["responses"] = acceptedResponses("StatusResponse")
	post := operation("post", "Admin", "Set a cached artifact action", adminSecurity(), "CacheArtifactActionRequest", "StatusResponse")
	post["parameters"] = del["parameters"]
	post["responses"] = acceptedResponses("StatusResponse")
	return map[string]any{"delete": del, "post": post}
}

func adminUsersOperation() map[string]any {
	return map[string]any{
		"get":  operation("get", "Admin", "List API clients", adminSecurity(), "", "UserList"),
		"post": operation("post", "Admin", "Create an API client", adminSecurity(), "CreateUserRequest", "User"),
		"put":  operation("put", "Admin", "Edit an API client", adminSecurity(), "UpdateUserRequest", "User"),
	}
}

func uploadOperation() map[string]any {
	op := operation("post", "Admin", "Upload an artifact file", adminSecurity(), "", "Artifact")
	op["requestBody"] = map[string]any{
		"required": true,
		"content": map[string]any{
			"multipart/form-data": map[string]any{"schema": ref("ArtifactUploadRequest")},
		},
	}
	return map[string]any{"post": op}
}

func getOperation(tag, summary string, security []map[string][]string, responseSchema string) map[string]any {
	return map[string]any{"get": operation("get", tag, summary, security, "", responseSchema)}
}

func postOperation(tag, summary string, security []map[string][]string, requestSchema, responseSchema string) map[string]any {
	return map[string]any{"post": operation("post", tag, summary, security, requestSchema, responseSchema)}
}

func deleteOperation(tag, summary string, security []map[string][]string, responseSchema string) map[string]any {
	return map[string]any{"delete": operation("delete", tag, summary, security, "", responseSchema)}
}

func operation(id, tag, summary string, security []map[string][]string, requestSchema, responseSchema string) map[string]any {
	out := map[string]any{
		"operationId": id + tag + operationIDSummary(summary),
		"tags":        []string{tag},
		"summary":     summary,
		"responses":   responses(responseSchema),
	}
	if len(security) > 0 {
		out["security"] = security
	}
	if requestSchema != "" {
		out["requestBody"] = jsonRequest(requestSchema)
	}
	return out
}

func operationIDSummary(summary string) string {
	out := ""
	nextUpper := true
	for _, r := range summary {
		if r < 'A' || r > 'z' || r > 'Z' && r < 'a' {
			nextUpper = true
			continue
		}
		if nextUpper && r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		out += string(r)
		nextUpper = false
	}
	return out
}

func responses(schemaName string) map[string]any {
	return map[string]any{
		"200": jsonResponse("OK", schemaName),
		"400": jsonResponse("Bad request", "ErrorResponse"),
		"401": jsonResponse("Unauthorized", "ErrorResponse"),
		"404": jsonResponse("Not found", "ErrorResponse"),
		"409": jsonResponse("Conflict", "ErrorResponse"),
		"503": jsonResponse("No capacity or unavailable", "ErrorResponse"),
	}
}

func acceptedResponses(schemaName string) map[string]any {
	out := responses(schemaName)
	out["202"] = jsonResponse("Accepted", schemaName)
	return out
}

func parameter(name, in string, required bool) map[string]any {
	return map[string]any{
		"name":     name,
		"in":       in,
		"required": required,
		"schema":   stringSchema(),
	}
}

func jsonRequest(schemaName string) map[string]any {
	return map[string]any{
		"required": true,
		"content":  map[string]any{"application/json": map[string]any{"schema": ref(schemaName)}},
	}
}

func jsonResponse(description, schemaName string) map[string]any {
	out := map[string]any{"description": description}
	if schemaName != "" {
		out["content"] = map[string]any{"application/json": map[string]any{"schema": ref(schemaName)}}
	}
	return out
}

func ref(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func adminSecurity() []map[string][]string {
	return []map[string][]string{{"AdminBearer": {}}}
}

func clientSecurity() []map[string][]string {
	return []map[string][]string{{"ClientBearer": {}}}
}

func workerSecurity() []map[string][]string {
	return []map[string][]string{{"WorkerBearer": {}}}
}

func openAPIComponents() map[string]any {
	return map[string]any{
		"securitySchemes": map[string]any{
			"AdminBearer":  bearerScheme("Manager admin token"),
			"ClientBearer": bearerScheme("User API key"),
			"WorkerBearer": bearerScheme("Worker enrollment token"),
		},
		"schemas": openAPISchemas(),
	}
}

func bearerScheme(description string) map[string]string {
	return map[string]string{"type": "http", "scheme": "bearer", "description": description}
}

func openAPISchemas() map[string]any {
	return map[string]any{
		"AdminBalanceAdjustmentRequest": object(map[string]any{"userId": stringSchema(), "amount": integerSchema(), "reason": stringSchema()}),
		"AdminStateWSTicketResponse":    object(map[string]any{"ticket": stringSchema(), "expiresAt": dateTimeSchema()}),
		"APIKeyList":                    arrayOf("APIKeyView"),
		"APIKeyView":                    object(map[string]any{"apiKeyId": stringSchema(), "userId": stringSchema(), "name": stringSchema(), "status": stringSchema()}),
		"APIKeyLookupRequest":           object(map[string]any{"token": stringSchema()}),
		"APIKeyLookupResponse":          object(map[string]any{"apiKey": ref("APIKeyView"), "user": ref("User")}),
		"ApplyWorkerUpdateRequest":      object(map[string]any{"kind": stringSchema(), "version": stringSchema(), "artifactId": stringSchema(), "sha256": stringSchema(), "signature": stringSchema(), "publicKey": stringSchema(), "targetNodes": arrayOfPrimitive("string")}),
		"Artifact":                      object(map[string]any{"artifactId": stringSchema(), "id": stringSchema(), "kind": stringSchema(), "name": stringSchema(), "sha256": stringSchema(), "sizeBytes": integerSchema(), "createdAt": dateTimeSchema(), "torrent": ref("ArtifactTorrent")}),
		"ArtifactTorrent":               object(map[string]any{"infoHash": stringSchema(), "magnetUri": stringSchema(), "pieceLength": integerSchema()}),
		"ArtifactList":                  arrayOf("Artifact"),
		"ArtifactUploadRequest":         object(map[string]any{"file": map[string]any{"type": "string", "format": "binary"}, "kind": stringSchema(), "name": stringSchema(), "sha256": stringSchema()}),
		"AttemptList":                   arrayOf("Attempt"),
		"Attempt":                       freeObject(),
		"BinaryArtifact":                map[string]any{"type": "string", "format": "binary"},
		"ChatCompletionChunk":           freeObject(),
		"ChatCompletionRequest":         object(map[string]any{"model": stringSchema(), "messages": arrayOf("ChatMessage"), "stream": map[string]string{"type": "boolean"}, "max_tokens": integerSchema(), "temperature": map[string]string{"type": "number"}, "min_context_tokens": integerSchema()}),
		"ChatMessage":                   object(map[string]any{"role": stringSchema(), "content": stringSchema()}),
		"CacheArtifactActionRequest":    object(map[string]any{"action": stringSchema()}),
		"ComputeLedgerEntry":            freeObject(),
		"CreateArtifactRequest":         object(map[string]any{"kind": stringSchema(), "name": stringSchema(), "path": stringSchema(), "sha256": stringSchema()}),
		"CreateProfileRequest":          freeObject(),
		"CreateUserRequest":             object(map[string]any{"userId": stringSchema(), "name": stringSchema()}),
		"ErrorResponse":                 object(map[string]any{"error": ref("ErrorBody")}),
		"ErrorBody":                     object(map[string]any{"code": stringSchema(), "message": stringSchema()}),
		"HTMLDocument":                  map[string]string{"type": "string"},
		"IssueAPIKeyRequest":            object(map[string]any{"userId": stringSchema(), "name": stringSchema()}),
		"ModelList":                     freeObject(),
		"NodeList":                      arrayOf("Node"),
		"Node":                          freeObject(),
		"OpenAPIDocument":               freeObject(),
		"PlainOK":                       map[string]string{"type": "string", "example": "OK"},
		"PlacementState":                freeObject(),
		"PlacementExplainResponse":      placementExplainResponseSchema(),
		"PlacementAssignment":           freeObject(),
		"PlacementProfileExplanation":   placementProfileExplanationSchema(),
		"PlacementCandidateExplanation": placementCandidateExplanationSchema(),
		"PlacementMissingExplanation":   placementMissingExplanationSchema(),
		"PolicyList":                    arrayOf("PlacementPolicy"),
		"PlacementPolicy":               placementPolicySchema(),
		"ProfileList":                   arrayOf("WorkloadProfile"),
		"PrometheusText":                map[string]string{"type": "string"},
		"ReportList":                    arrayOf("ComputeReport"),
		"ComputeReport":                 freeObject(),
		"RevokeAPIKeyRequest":           object(map[string]any{"apiKeyId": stringSchema()}),
		"SetProfileComputeCostRequest":  object(map[string]any{"profileId": stringSchema(), "computeCost": integerSchema()}),
		"SlotList":                      arrayOf("Slot"),
		"Slot":                          freeObject(),
		"StateResponse":                 freeObject(),
		"StatusResponse":                object(map[string]any{"status": stringSchema()}),
		"TaskListResponse":              freeObject(),
		"Task":                          freeObject(),
		"UnbanRequest":                  object(map[string]any{"nodeId": stringSchema(), "slotId": stringSchema()}),
		"UpdateList":                    arrayOf("UpdateRecord"),
		"UpdateNodeRequest":             object(map[string]any{"nodeId": stringSchema(), "ownerUserId": stringSchema(), "approved": map[string]string{"type": "boolean"}, "mode": stringSchema(), "tags": arrayOfPrimitive("string"), "state": stringSchema()}),
		"UpdateRecord":                  freeObject(),
		"UpdateUserRequest":             object(map[string]any{"userId": stringSchema(), "name": stringSchema(), "disabled": map[string]string{"type": "boolean"}}),
		"UpsertPolicyRequest":           placementPolicySchema(),
		"User":                          object(map[string]any{"userId": stringSchema(), "name": stringSchema(), "computeBalance": integerSchema(), "disabled": map[string]string{"type": "boolean"}, "createdAt": dateTimeSchema()}),
		"UserList":                      arrayOf("User"),
		"WebSocketUpgrade":              map[string]string{"type": "string", "description": "HTTP upgrade to WebSocket"},
		"WorkerJoinResponse":            object(map[string]any{"managerUrl": stringSchema(), "workerToken": stringSchema(), "installCommand": stringSchema()}),
		"WorkloadProfile":               freeObject(),
		"YAMLDocument":                  map[string]string{"type": "string"},
	}
}

func placementPolicySchema() map[string]any {
	return object(map[string]any{
		"policyId":                 stringSchema(),
		"profileId":                stringSchema(),
		"cachedCount":              integerSchema(),
		"warmCount":                integerSchema(),
		"autoBalance":              booleanSchema(),
		"minCachedCount":           integerSchema(),
		"maxCachedCount":           integerSchema(),
		"minWarmCount":             integerSchema(),
		"maxWarmCount":             integerSchema(),
		"maxCachedProfilesPerNode": integerSchema(),
		"maxWarmProfilesPerNode":   integerSchema(),
		"effectiveCachedCount":     integerSchema(),
		"effectiveWarmCount":       integerSchema(),
		"demandQueued":             integerSchema(),
		"demandRunning":            integerSchema(),
		"demandRecent":             integerSchema(),
		"demandSmoothed":           integerSchema(),
		"constraints":              freeObject(),
		"hardPinnedSlots":          arrayOfPrimitive("string"),
		"createdAt":                dateTimeSchema(),
		"updatedAt":                dateTimeSchema(),
		"conditions":               map[string]any{"type": "array", "items": freeObject()},
	})
}

func object(properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
}

func freeObject() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": true}
}

func arrayOf(schemaName string) map[string]any {
	return map[string]any{"type": "array", "items": ref(schemaName)}
}

func arrayOfPrimitive(kind string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]string{"type": kind}}
}

func stringSchema() map[string]string {
	return map[string]string{"type": "string"}
}

func integerSchema() map[string]string {
	return map[string]string{"type": "integer", "format": "int64"}
}

func booleanSchema() map[string]string {
	return map[string]string{"type": "boolean"}
}

func dateTimeSchema() map[string]string {
	return map[string]string{"type": "string", "format": "date-time"}
}

func openAPIDocsHTML() string {
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>COMRAD API Reference</title>
  <style>
    :root { color-scheme: dark; --bg:#09090b; --panel:#121216; --soft:#1d1d23; --line:#2a2a33; --text:#f4f4f5; --muted:#a1a1aa; --accent:#7dd3fc; --green:#86efac; --yellow:#fde68a; }
    * { box-sizing: border-box; }
    body { margin:0; background:var(--bg); color:var(--text); font:14px/1.5 Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    header { position:sticky; top:0; z-index:2; border-bottom:1px solid var(--line); background:rgba(9,9,11,.92); backdrop-filter: blur(14px); }
    .wrap { max-width:1180px; margin:0 auto; padding:24px; }
    h1 { margin:0; font-size:28px; letter-spacing:0; }
    h2 { margin:34px 0 12px; font-size:18px; letter-spacing:0; }
    p { margin:6px 0 0; color:var(--muted); }
    a { color:var(--accent); text-decoration:none; }
    code { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; color:#e4e4e7; }
    .toolbar { display:flex; gap:12px; align-items:center; justify-content:space-between; flex-wrap:wrap; }
    .search { min-width:min(100%,420px); padding:10px 12px; border-radius:10px; border:1px solid var(--line); background:var(--panel); color:var(--text); outline:none; }
    .grid { display:grid; gap:12px; }
    .endpoint { border:1px solid var(--line); border-radius:12px; background:var(--panel); padding:16px; }
    .top { display:flex; gap:10px; align-items:center; justify-content:space-between; flex-wrap:wrap; }
    .method { display:inline-flex; min-width:64px; justify-content:center; border-radius:8px; padding:4px 8px; font-weight:700; background:var(--soft); color:var(--green); }
    .post { color:var(--accent); } .delete { color:#fca5a5; }
    .path { font-size:15px; font-weight:600; }
    .meta { display:flex; gap:8px; flex-wrap:wrap; margin-top:10px; }
    .pill { border:1px solid var(--line); border-radius:999px; padding:3px 8px; color:var(--muted); font-size:12px; }
    .summary { margin-top:12px; color:#d4d4d8; }
    .schema { display:none; margin-top:12px; padding:12px; border-radius:10px; background:#0d0d10; border:1px solid var(--line); color:var(--muted); white-space:pre-wrap; overflow:auto; }
    .endpoint.open .schema { display:block; }
    button { border:1px solid var(--line); background:var(--soft); color:var(--text); border-radius:9px; padding:7px 10px; cursor:pointer; }
    .notice { border:1px solid var(--line); background:#111118; border-radius:12px; padding:14px; color:var(--muted); }
  </style>
</head>
<body>
  <header>
    <div class="wrap toolbar">
      <div>
        <h1>COMRAD API Reference</h1>
        <p>OpenAPI 3.1 · admin-only · <a id="specLink" href="/api/admin/openapi.json">/api/admin/openapi.json</a></p>
      </div>
      <input id="search" class="search" autocomplete="off" placeholder="Filter endpoints, schemas, or tags" />
    </div>
  </header>
  <main class="wrap">
    <div class="notice">Use <code>Authorization: Bearer &lt;token&gt;</code>. Admin endpoints require the admin token; client endpoints require a client API key; Worker endpoints require the Worker token.</div>
    <div id="content" class="grid"></div>
  </main>
  <script>
    const content = document.getElementById("content");
    const search = document.getElementById("search");
    const token = localStorage.getItem("comrad.adminToken") || "";
    const headers = token ? { Authorization: "Bearer " + token } : {};
    let endpoints = [];

    fetch("/api/admin/openapi.json", { headers }).then(async (res) => {
      if (!res.ok) throw new Error("OpenAPI fetch failed: " + res.status);
      const spec = await res.json();
      endpoints = Object.entries(spec.paths).flatMap(([path, item]) =>
        Object.entries(item).map(([method, op]) => ({ path, method, op })));
      render();
    }).catch((err) => {
      content.innerHTML = '<div class="notice">' + err.message + '</div>';
    });

    search.addEventListener("input", render);

    function render() {
      const q = search.value.toLowerCase();
      const filtered = endpoints.filter((e) => textFor(e).includes(q));
      const groups = groupByTag(filtered);
      content.innerHTML = Object.entries(groups).map(([tag, items]) =>
        '<section><h2>' + tag + '</h2><div class="grid">' + items.map(endpointHTML).join("") + '</div></section>'
      ).join("");
      document.querySelectorAll(".endpoint button").forEach((button) => {
        button.addEventListener("click", () => button.closest(".endpoint").classList.toggle("open"));
      });
    }

    function groupByTag(items) {
      return items.reduce((acc, item) => {
        const tag = (item.op.tags && item.op.tags[0]) || "Other";
        (acc[tag] ||= []).push(item);
        return acc;
      }, {});
    }

    function endpointHTML(item) {
      const auth = (item.op.security || []).flatMap(Object.keys).join(", ") || "public";
      const req = schemaName(item.op.requestBody);
      const response = schemaName(item.op.responses && item.op.responses["200"]);
      return '<article class="endpoint">' +
        '<div class="top"><div><span class="method ' + item.method + '">' + item.method.toUpperCase() + '</span> <code class="path">' + item.path + '</code></div><button>Details</button></div>' +
        '<div class="summary">' + escapeHTML(item.op.summary || "") + '</div>' +
        '<div class="meta"><span class="pill">' + auth + '</span><span class="pill">request: ' + (req || "none") + '</span><span class="pill">response: ' + (response || "none") + '</span></div>' +
        '<pre class="schema">' + escapeHTML(JSON.stringify(item.op, null, 2)) + '</pre>' +
      '</article>';
    }

    function schemaName(value) {
      const schema = value?.content?.["application/json"]?.schema || value?.content?.["multipart/form-data"]?.schema;
      return schema?.$ref ? schema.$ref.split("/").pop() : "";
    }

    function textFor(item) {
      return [item.path, item.method, item.op.summary, ...(item.op.tags || [])].join(" ").toLowerCase();
    }

    function escapeHTML(value) {
      return String(value).replace(/[&<>"']/g, (c) => ({ "&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;", "'":"&#39;" }[c]));
    }
  </script>
</body>
</html>`
}
