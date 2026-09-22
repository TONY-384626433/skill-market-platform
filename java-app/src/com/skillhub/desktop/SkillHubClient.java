package com.skillhub.desktop;

import java.net.URI;
import java.net.URLEncoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.Map;

/**
 * SkillHub 后端 API 客户端 (java.net.http, 零第三方依赖)。
 */
public class SkillHubClient {
    private final HttpClient http = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(10))
            .followRedirects(HttpClient.Redirect.NORMAL)
            .build();
    private String base;

    public SkillHubClient(String base) { setBase(base); }

    public final void setBase(String base) {
        String b = base == null ? "" : base.trim();
        while (b.endsWith("/")) b = b.substring(0, b.length() - 1);
        this.base = b;
    }

    public String getBase() { return base; }

    private String get(String path) throws Exception {
        HttpRequest req = HttpRequest.newBuilder(URI.create(base + path))
                .timeout(Duration.ofSeconds(45))
                .header("Accept", "application/json")
                .GET()
                .build();
        HttpResponse<String> resp = http.send(req, HttpResponse.BodyHandlers.ofString(StandardCharsets.UTF_8));
        if (resp.statusCode() >= 400) {
            throw new RuntimeException("HTTP " + resp.statusCode() + " — " + truncate(resp.body()));
        }
        return resp.body();
    }

    private static String truncate(String s) {
        if (s == null) return "";
        return s.length() > 240 ? s.substring(0, 240) + "…" : s;
    }

    private static String enc(String v) {
        return URLEncoder.encode(v == null ? "" : v, StandardCharsets.UTF_8);
    }

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
}
