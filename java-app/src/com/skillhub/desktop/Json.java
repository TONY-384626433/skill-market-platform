package com.skillhub.desktop;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * 极简 JSON 解析器 (无第三方依赖)。
 * 支持 object / array / string / number / boolean / null。
 * 解析结果：Map&lt;String,Object&gt; / List&lt;Object&gt; / String / Long/Double / Boolean / null
 */
public final class Json {
    private final String s;
    private int i;

    private Json(String s) { this.s = s; }

    public static Object parse(String text) {
        Json p = new Json(text == null ? "null" : text);
        p.ws();
        return p.value();
    }

    @SuppressWarnings("unchecked")
    public static Map<String, Object> asObject(Object v) {
        return v instanceof Map ? (Map<String, Object>) v : new LinkedHashMap<>();
    }

    @SuppressWarnings("unchecked")
    public static List<Object> asArray(Object v) {
        return v instanceof List ? (List<Object>) v : new ArrayList<>();
    }

    public static String str(Object v) { return v == null ? "" : String.valueOf(v); }

    private void ws() { while (i < s.length() && Character.isWhitespace(s.charAt(i))) i++; }

    private Object value() {
        ws();
        if (i >= s.length()) return null;
        char c = s.charAt(i);
        switch (c) {
            case '{': return obj();
            case '[': return arr();
            case '"': return str();
            case 't': i += 4; return Boolean.TRUE;
            case 'f': i += 5; return Boolean.FALSE;
            case 'n': i += 4; return null;
            default: return num();
        }
    }

    private Map<String, Object> obj() {
        Map<String, Object> m = new LinkedHashMap<>();
        i++; ws();
        if (i < s.length() && s.charAt(i) == '}') { i++; return m; }
        while (i < s.length()) {
            ws();
            String k = str();
            ws();
            if (i < s.length() && s.charAt(i) == ':') i++;
            m.put(k, value());
            ws();
            if (i >= s.length()) break;
            char c = s.charAt(i++);
            if (c == '}') break;
        }
        return m;
    }

    private List<Object> arr() {
        List<Object> l = new ArrayList<>();
        i++; ws();
        if (i < s.length() && s.charAt(i) == ']') { i++; return l; }
        while (i < s.length()) {
            l.add(value());
            ws();
            if (i >= s.length()) break;
            char c = s.charAt(i++);
            if (c == ']') break;
        }
        return l;
    }

    private String str() {
        StringBuilder b = new StringBuilder();
        i++; // skip opening quote
        while (i < s.length()) {
            char c = s.charAt(i++);
            if (c == '"') break;
            if (c == '\\' && i < s.length()) {
                char e = s.charAt(i++);
                switch (e) {
                    case 'n': b.append('\n'); break;
                    case 't': b.append('\t'); break;
                    case 'r': b.append('\r'); break;
                    case 'b': b.append('\b'); break;
                    case 'f': b.append('\f'); break;
                    case '/': b.append('/'); break;
                    case '"': b.append('"'); break;
                    case '\\': b.append('\\'); break;
                    case 'u':
                        if (i + 4 <= s.length()) {
                            b.append((char) Integer.parseInt(s.substring(i, i + 4), 16));
                            i += 4;
                        }
                        break;
                    default: b.append(e);
                }
            } else {
                b.append(c);
            }
        }
        return b.toString();
    }

    private Object num() {
        int st = i;
        while (i < s.length() && "-+.eE0123456789".indexOf(s.charAt(i)) >= 0) i++;
        String t = s.substring(st, i);
        if (t.isEmpty()) return null;
        if (t.contains(".") || t.contains("e") || t.contains("E")) return Double.parseDouble(t);
        try { return Long.parseLong(t); } catch (NumberFormatException ex) { return Double.parseDouble(t); }
    }
}
