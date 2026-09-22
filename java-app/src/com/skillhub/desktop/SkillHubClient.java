package com.skillhub.desktop;

import java.net.URI;
import java.net.URLEncoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.List;
import java.util.Map;

/**
 * SkillHub 后端 API 客户端 (java.net.http, 零第三方依赖)。
 * 支持：健康检查 / 技能检索 / 登录 / Agent 对话(编排技能)。
 */
public class SkillHubClient {
    private final HttpClient http = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(10))
            .followRedirects(HttpClient.Redirect.NORMAL)
            .build();
    private String base;

    private String token;
    private String username;
    private String role;
    private String preferredProvider = "";

    public SkillHubClient(String base) { setBase(base); }

    public final void setBase(String base) {
        String b = base == null ? "" : base.trim();
        while (b.endsWith("/")) b = b.substring(0, b.length() - 1);
        this.base = b;
    }

    public String getBase() { return base; }

    public boolean isLoggedIn() { return token != null && !token.isEmpty(); }
    public String getUsername() { return username; }
    public String getRole() { return role; }
    public void setPreferredProvider(String k) { this.preferredProvider = k == null ? "" : k.trim(); }
    public String getPreferredProvider() { return preferredProvider; }
    public void logout() { token = null; username = null; role = null; }

    private static String enc(String v) {
        return URLEncoder.encode(v == null ? "" : v, StandardCharsets.UTF_8);
    }

    // ---- 统一请求（自动带 Bearer Token） ----
    private HttpResponse<String> send(String method, String path, String jsonBody, int timeoutSec) throws Exception {
        HttpRequest.Builder b = HttpRequest.newBuilder(URI.create(base + path))
                .timeout(Duration.ofSeconds(timeoutSec))
                .header("Accept", "application/json");
        if (isLoggedIn()) b.header("Authorization", "Bearer " + token);
        if (jsonBody != null) {
            b.header("Content-Type", "application/json");
            b.method(method, HttpRequest.BodyPublishers.ofString(jsonBody, StandardCharsets.UTF_8));
        } else {
            b.method(method, HttpRequest.BodyPublishers.noBody());
        }
        return http.send(b.build(), HttpResponse.BodyHandlers.ofString(StandardCharsets.UTF_8));
    }

    private String get(String path) throws Exception {
        HttpResponse<String> resp = send("GET", path, null, 45);
        if (resp.statusCode() >= 400) throw new RuntimeException("HTTP " + resp.statusCode() + " — " + truncate(resp.body()));
        return resp.body();
    }

    private static String truncate(String s) {
        if (s == null) return "";
        return s.length() > 240 ? s.substring(0, 240) + "…" : s;
    }

    private static String esc(String s) {
        if (s == null) return "";
        StringBuilder b = new StringBuilder();
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            switch (c) {
                case '"': b.append("\\\""); break;
                case '\\': b.append("\\\\"); break;
                case '\n': b.append("\\n"); break;
                case '\r': b.append("\\r"); break;
                case '\t': b.append("\\t"); break;
                default: b.append(c);
            }
        }
        return b.toString();
    }

    // ---- 基础接口 ----
    public Map<String, Object> health() throws Exception {
        return Json.asObject(Json.parse(get("/health")));
    }

    public Map<String, Object> stats() throws Exception {
        return Json.asObject(Json.parse(get("/skills/stats/overview")));
    }

    public Map<String, Object> searchInternal(String query, int page, int size) throws Exception {
        return Json.asObject(Json.parse(get("/skills?query=" + enc(query) + "&page=" + page + "&page_size=" + size)));
    }

    public Map<String, Object> searchGitHub(String query, int page, int size) throws Exception {
        return Json.asObject(Json.parse(get("/github/skills/search?query=" + enc(query) + "&page=" + page + "&page_size=" + size)));
    }

    // ---- 登录 ----
    public Map<String, Object> login(String user, String pass) throws Exception {
        String body = "{\"username\":\"" + esc(user) + "\",\"password\":\"" + esc(pass) + "\"}";
        HttpResponse<String> resp = send("POST", "/auth/login", body, 30);
        if (resp.statusCode() >= 400) {
            throw new RuntimeException("登录失败 HTTP " + resp.statusCode() + " — " + truncate(resp.body()));
        }
        Map<String, Object> m = Json.asObject(Json.parse(resp.body()));
        this.token = Json.str(m.get("token"));
        Map<String, Object> u = Json.asObject(m.get("user"));
        this.username = Json.str(u.get("username"));
        this.role = Json.str(u.get("role"));
        return m;
    }

    // ---- Agent: 自然语言对话并自动编排技能 ----
    public Map<String, Object> agentStatus() throws Exception {
        return Json.asObject(Json.parse(get("/agent/status")));
    }

    /** 大模型厂商列表 (需登录) */
    public java.util.List<Object> listProviders() throws Exception {
        Map<String, Object> root = Json.asObject(Json.parse(get("/agent/providers")));
        return Json.asArray(root.get("data"));
    }

    /** history 元素: {"role":"user|assistant","content":"..."} */
    public Map<String, Object> agentChat(String message, List<Map<String, String>> history) throws Exception {
        StringBuilder sb = new StringBuilder("{\"message\":\"").append(esc(message)).append("\"");
        if (preferredProvider != null && !preferredProvider.isEmpty()) {
            sb.append(",\"provider\":\"").append(esc(preferredProvider)).append("\"");
        }
        if (history != null && !history.isEmpty()) {
            sb.append(",\"history\":[");
            for (int i = 0; i < history.size(); i++) {
                Map<String, String> h = history.get(i);
                if (i > 0) sb.append(',');
                sb.append("{\"role\":\"").append(esc(h.get("role")))
                  .append("\",\"content\":\"").append(esc(h.get("content"))).append("\"}");
            }
            sb.append(']');
        }
        sb.append('}');
        HttpResponse<String> resp = send("POST", "/agent/chat", sb.toString(), 200);
        if (resp.statusCode() >= 400) {
            throw new RuntimeException("Agent HTTP " + resp.statusCode() + " — " + truncate(resp.body()));
        }
        Map<String, Object> root = Json.asObject(Json.parse(resp.body()));
        return Json.asObject(root.get("data"));
    }
}
