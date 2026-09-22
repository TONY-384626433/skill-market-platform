package com.skillhub.desktop;

import javax.swing.*;
import javax.swing.border.EmptyBorder;
import java.awt.*;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * 微信风格聊天面板 (Swing 原生绘制)
 * — 左：会话列表；右：消息气泡；底：输入框 + 发送。
 * 发消息会调用 SkillHub 后端检索技能并回复。
 */
public class ChatPanel extends JPanel {

    private static final Color SIDEBAR_BG = new Color(0x2E2E2E);
    private static final Color SIDEBAR_SEL = new Color(0x3D3D3D);
    private static final Color CHAT_BG = new Color(0xF1F1F1);
    private static final Color MY_BUBBLE = new Color(0x95EC69);
    private static final Color OTHER_BUBBLE = Color.WHITE;
    private static final Color BRAND = new Color(0x07C160);

    private final SkillHubClient client;

    private final DefaultListModel<Contact> contactModel = new DefaultListModel<>();
    private final JList<Contact> contactList = new JList<>(contactModel);
    private final JPanel messageList = new JPanel();
    private final JScrollPane messageScroll = new JScrollPane(messageList);
    private final JTextField input = new JTextField();
    private final JLabel chatTitle = new JLabel();
    private final JLabel chatSub = new JLabel();
    private final JButton authBtn = new JButton("登录 Agent");
    private final JComboBox<String> providerBox = new JComboBox<>();
    private final java.util.List<String> providerKeys = new ArrayList<>();
    private boolean updatingProviders = false;
    private Runnable onLoginRequested;

    /** 每个会话独立保存消息: role(text) -> "me"/"ai" */
    private final Map<String, List<String[]>> conversations = new LinkedHashMap<>();

    private Contact active;

    private static class Contact {
        final String name;
        final String avatarText;
        final Color color;
        String preview = "";
        int unread = 0;
        Contact(String name, String avatarText, Color color) {
            this.name = name; this.avatarText = avatarText; this.color = color;
        }
        @Override public String toString() { return name; }
    }

    public ChatPanel(SkillHubClient client) {
        this.client = client;
        setLayout(new BorderLayout());

        contactModel.addElement(new Contact("AI 技能助手", "AI", new Color(0x07C160)));
        contactModel.addElement(new Contact("安全治理小助手", "盾", new Color(0x576B95)));
        contactModel.addElement(new Contact("运维巡检 Bot", "运", new Color(0xE6A23C)));

        add(buildSidebar(), BorderLayout.WEST);
        add(buildChat(), BorderLayout.CENTER);

        contactList.addListSelectionListener(e -> {
            if (!e.getValueIsAdjusting() && contactList.getSelectedValue() != null) switchTo(contactList.getSelectedValue());
        });
        contactList.setSelectedIndex(0);
        if (active == null && contactModel.size() > 0) switchTo(contactModel.getElementAt(0));
    }

    /** 供命令行 --demo 使用：自动发一条消息，便于验证/截图 */
    public void demoSend(String text) {
        input.setText(text);
        send();
    }

    // ---------------- 左侧会话列表 ----------------
    private JComponent buildSidebar() {
        JPanel side = new JPanel(new BorderLayout());
        side.setPreferredSize(new Dimension(230, 0));
        side.setBackground(SIDEBAR_BG);

        JPanel head = new JPanel(new BorderLayout());
        head.setBackground(new Color(0x252525));
        head.setPreferredSize(new Dimension(0, 56));
        JLabel t = new JLabel("  SkillHub 会话");
        t.setForeground(new Color(0xE8E8E8));
        t.setFont(t.getFont().deriveFont(Font.BOLD, 15f));
        head.add(t, BorderLayout.CENTER);
        side.add(head, BorderLayout.NORTH);

        contactList.setCellRenderer(new ContactRenderer());
        contactList.setBackground(SIDEBAR_BG);
        contactList.setSelectionMode(ListSelectionModel.SINGLE_SELECTION);
        contactList.setFixedCellHeight(64);
        contactList.setBorder(new EmptyBorder(6, 0, 0, 0));
        side.add(contactList, BorderLayout.CENTER);
        return side;
    }

    private class ContactRenderer extends JPanel implements ListCellRenderer<Contact> {
        private final JLabel avatar = new JLabel();
        private final JLabel name = new JLabel();
        private final JLabel preview = new JLabel();
        ContactRenderer() {
            setLayout(new BorderLayout(10, 0));
            setBorder(new EmptyBorder(8, 12, 8, 12));
            avatar.setPreferredSize(new Dimension(42, 42));
            JPanel texts = new JPanel(new GridLayout(2, 1, 0, 2));
            texts.setOpaque(false);
            name.setFont(name.getFont().deriveFont(Font.PLAIN, 14f));
            preview.setFont(preview.getFont().deriveFont(Font.PLAIN, 12f));
            texts.add(name); texts.add(preview);
            add(avatar, BorderLayout.WEST);
            add(texts, BorderLayout.CENTER);
        }
        @Override public Component getListCellRendererComponent(JList<? extends Contact> list, Contact c, int index, boolean selected, boolean focus) {
            setBackground(selected ? SIDEBAR_SEL : SIDEBAR_BG);
            setOpaque(true);
            avatar.setIcon(new AvatarIcon(c.avatarText, c.color, 42));
            name.setForeground(new Color(0xEDEDED));
            preview.setForeground(new Color(0x9A9A9A));
            name.setText(c.name);
            preview.setText(c.preview.isEmpty() ? "点击开始对话" : c.preview);
            return this;
        }
    }

    // ---------------- 右侧聊天区 ----------------
    private JComponent buildChat() {
        JPanel chat = new JPanel(new BorderLayout());
        chat.setBackground(CHAT_BG);

        JPanel header = new JPanel(new BorderLayout());
        header.setBackground(new Color(0xF7F7F7));
        header.setBorder(new EmptyBorder(8, 16, 8, 16));
        header.setPreferredSize(new Dimension(0, 56));
        chatTitle.setFont(chatTitle.getFont().deriveFont(Font.BOLD, 15f));
        chatSub.setFont(chatSub.getFont().deriveFont(Font.PLAIN, 11f));
        chatSub.setForeground(new Color(0x888888));
        JPanel htext = new JPanel(new GridLayout(2, 1));
        htext.setOpaque(false);
        htext.add(chatTitle); htext.add(chatSub);
        header.add(htext, BorderLayout.WEST);
        authBtn.setFocusPainted(false);
        authBtn.setFont(authBtn.getFont().deriveFont(Font.PLAIN, 12f));
        authBtn.addActionListener(e -> { if (onLoginRequested != null) onLoginRequested.run(); });
        providerBox.setFont(providerBox.getFont().deriveFont(Font.PLAIN, 12f));
        providerBox.setPreferredSize(new Dimension(170, 26));
        providerBox.setEnabled(false);
        providerBox.addItem("(登录后可选厂商)");
        providerBox.addActionListener(e -> {
            int idx = providerBox.getSelectedIndex();
            if (!updatingProviders && idx >= 0 && idx < providerKeys.size()) client.setPreferredProvider(providerKeys.get(idx));
        });
        JPanel hright = new JPanel(new FlowLayout(FlowLayout.RIGHT, 6, 0));
        hright.setOpaque(false);
        hright.add(providerBox);
        hright.add(authBtn);
        header.add(hright, BorderLayout.EAST);
        chat.add(header, BorderLayout.NORTH);

        messageList.setLayout(new BoxLayout(messageList, BoxLayout.Y_AXIS));
        messageList.setBackground(CHAT_BG);
        messageScroll.setBorder(null);
        messageScroll.getViewport().setBackground(CHAT_BG);
        messageScroll.getVerticalScrollBar().setUnitIncrement(16);
        chat.add(messageScroll, BorderLayout.CENTER);

        JPanel inputBar = new JPanel(new BorderLayout(10, 0));
        inputBar.setBackground(new Color(0xF7F7F7));
        inputBar.setBorder(new EmptyBorder(10, 14, 12, 14));
        input.setFont(input.getFont().deriveFont(Font.PLAIN, 14f));
        input.setBackground(Color.WHITE);
        input.setBorder(BorderFactory.createCompoundBorder(
                BorderFactory.createLineBorder(new Color(0xDDDDDD)),
                new EmptyBorder(8, 10, 8, 10)));
        input.addActionListener(e -> send());
        JButton send = new RoundButton("发送", BRAND);
        send.setPreferredSize(new Dimension(88, 38));
        send.addActionListener(e -> send());
        inputBar.add(input, BorderLayout.CENTER);
        inputBar.add(send, BorderLayout.EAST);
        chat.add(inputBar, BorderLayout.SOUTH);
        return chat;
    }

    private void switchTo(Contact c) {
        active = c;
        c.unread = 0;
        chatTitle.setText(c.name);
        chatSub.setText(client.isLoggedIn() ? "在线 · Agent 已接入" : "在线 · 技能检索模式（登录后可接入 Agent）");
        messageList.removeAll();
        List<String[]> conv = conversations.computeIfAbsent(c.name, k -> new ArrayList<>());
        if (conv.isEmpty()) {
            conv.add(new String[]{"ai", "你好，我是 " + c.name + " 👋\n告诉我你要找什么技能，我帮你搜～"});
        }
        for (String[] m : conv) render(m[0], m[1], false);
        contactList.repaint();
        messageList.revalidate();
        messageList.repaint();
        input.requestFocusInWindow();
    }

    private void send() {
        String text = input.getText().trim();
        if (text.isEmpty() || active == null) return;
        input.setText("");
        conversations.computeIfAbsent(active.name, k -> new ArrayList<>()).add(new String[]{"me", text});
        render("me", text, true);
        active.preview = text;
        respond(text);
    }

    private void respond(String text) {
        final Contact target = active;
        render("ai", client.isLoggedIn() ? "Agent 编排中…" : "正在检索…", true);
        new SwingWorker<String, Void>() {
            @Override protected String doInBackground() {
                try {
                    if (client.isLoggedIn()) return callAgent(text, target);
                    String kw = toKeyword(text);
                    Map<String, Object> res = client.searchGitHub(kw, 1, 4);
                    List<Object> data = Json.asArray(res.get("data"));
                    if (data.isEmpty()) {
                        // 回退 1: 企业审核库
                        Map<String, Object> inner = client.searchInternal(kw, 1, 4);
                        List<Object> idata = Json.asArray(inner.get("data"));
                        if (!idata.isEmpty()) {
                            StringBuilder sb = new StringBuilder("开源没找到，但企业审核库里有 " + Json.str(inner.get("total")) + " 个：\n");
                            int i = 1;
                            for (Object o : idata) {
                                Map<String, Object> m = Json.asObject(o);
                                sb.append(i++).append(". ").append(Json.str(m.get("name")))
                                  .append("  [").append(Json.str(m.get("category"))).append("]").append('\n');
                            }
                            return sb.toString();
                        }
                        // 回退 2: 换用第一个词再试一次
                        String first = kw.split("\\s+")[0];
                        if (!first.isEmpty() && !first.equals(kw)) {
                            Map<String, Object> r2 = client.searchGitHub(first, 1, 4);
                            List<Object> d2 = Json.asArray(r2.get("data"));
                            if (!d2.isEmpty()) return formatHits(r2, d2, "换个词（" + first + "）帮你找到");
                        }
                        return "没搜到相关技能～ 换个更具体的英文/关键词试试？\n例如：log desensitize、security audit、data analysis";
                    }
                    return formatHits(res, data, "帮你找到");
                } catch (Exception ex) {
                    return "连接后端失败：" + ex.getMessage() + "\n请检查「技能市场」页里的后端地址。";
                }
            }
            @Override protected void done() {
                try {
                    String reply = get();
                    conversations.computeIfAbsent(target.name, k -> new ArrayList<>()).add(new String[]{"ai", reply});
                    removeLastMessage();
                    render("ai", reply, true);
                    target.preview = reply.split("\n")[0];
                    contactList.repaint();
                } catch (Exception ex) {
                    removeLastMessage();
                    render("ai", "出错了：" + ex.getMessage(), true);
                }
            }
        }.execute();
    }

    /** 由外部(App)设置登录动作 */
    public void setOnLoginRequested(Runnable r) { this.onLoginRequested = r; }

    /** 登录状态变化后刷新头部按钮/副标题 */
    public void refreshAuth() {
        authBtn.setText(client.isLoggedIn() ? ("已登录: " + (client.getUsername() == null ? "—" : client.getUsername())) : "登录 Agent");
        if (active != null) chatSub.setText(client.isLoggedIn() ? "在线 · Agent 已接入" : "在线 · 技能检索模式（登录后可接入 Agent）");
        refreshProviders();
    }

    /** 拉取大模型厂商列表并填入选框 */
    public void refreshProviders() {
        if (!client.isLoggedIn()) {
            updatingProviders = true;
            providerBox.removeAllItems();
            providerKeys.clear();
            providerBox.addItem("(登录后可选厂商)");
            providerBox.setEnabled(false);
            updatingProviders = false;
            return;
        }
        providerBox.setEnabled(true);
        new SwingWorker<List<Object>, Void>() {
            @Override protected List<Object> doInBackground() throws Exception { return client.listProviders(); }
            @Override protected void done() {
                try {
                    List<Object> data = get();
                    updatingProviders = true;
                    providerBox.removeAllItems();
                    providerKeys.clear();
                    for (Object o : data) {
                        Map<String, Object> m = Json.asObject(o);
                        boolean configured = Boolean.TRUE.equals(m.get("configured"));
                        providerKeys.add(Json.str(m.get("key")));
                        providerBox.addItem(Json.str(m.get("label")) + (configured ? "" : " ·未配置"));
                    }
                    updatingProviders = false;
                    if (!providerKeys.isEmpty()) {
                        providerBox.setSelectedIndex(0);
                        client.setPreferredProvider(providerKeys.get(0));
                    }
                } catch (Exception ignored) { updatingProviders = false; }
            }
        }.execute();
    }

    /** 登录后走真 Agent: 自动编排并调用技能 */
    private String callAgent(String text, Contact target) {
        try {
            List<Map<String, String>> history = new ArrayList<>();
            List<String[]> conv = conversations.get(target.name);
            if (conv != null) {
                // 排除最后一条(即当前用户消息)，其余作为上下文；只取最近 6 轮
                int end = Math.max(0, conv.size() - 1);
                int start = Math.max(0, end - 12);
                for (int i = start; i < end; i++) {
                    String[] m = conv.get(i);
                    Map<String, String> h = new LinkedHashMap<>();
                    h.put("role", "me".equals(m[0]) ? "user" : "assistant");
                    h.put("content", m[1]);
                    history.add(h);
                }
            }
            Map<String, Object> answer = client.agentChat(text, history);
            String a = Json.str(answer.get("answer"));
            int calls = (int) toLong(answer.get("tool_calls"));
            String provider = Json.str(answer.get("provider"));
            String model = Json.str(answer.get("model"));
            StringBuilder sb = new StringBuilder(a.isEmpty() ? "（Agent 无回复）" : a);
            if (calls > 0) sb.append("\n\n🔧 自动编排并调用了 ").append(calls).append(" 个技能");
            sb.append("\n— 引擎: ").append(provider.isEmpty() ? "agent" : provider);
            if (!model.isEmpty()) sb.append(" · ").append(model);
            return sb.toString();
        } catch (Exception ex) {
            return "Agent 调用失败：" + ex.getMessage() + "\n可试试先在「技能市场」页确认后端地址(勾), 或重新登录。";
        }
    }

    private static long toLong(Object v) {
        if (v instanceof Number) return ((Number) v).longValue();
        try { return Long.parseLong(String.valueOf(v)); } catch (Exception e) { return 0; }
    }

    private static String formatHits(Map<String, Object> res, List<Object> data, String lead) {
        StringBuilder sb = new StringBuilder(lead + " " + Json.str(res.get("total")) + " 个相关技能：\n");
        int i = 1;
        for (Object o : data) {
            Map<String, Object> m = Json.asObject(o);
            sb.append(i++).append(". ").append(Json.str(m.get("name")))
              .append("  @").append(Json.str(m.get("repository"))).append('\n');
        }
        sb.append("\n（在「技能市场」页双击某行可打开仓库）");
        return sb.toString();
    }

    /** 从自然语言里抽取检索关键词：去口语、中文→英文提示词 */
    static String toKeyword(String text) {
        String t = text == null ? "" : text;
        String[] fillers = {"帮我找", "帮我", "请帮我", "请", "找一下", "找找", "搜索", "搜一下", "查一下", "查查",
                "有没有", "我要", "我想", "想要", "麻烦", "相关的", "相关", "的", "技能", "工具", "一个"};
        for (String f : fillers) t = t.replace(f, " ");
        t = t.replaceAll("[，。！？、,.!?;；:：（）()【】\\[\\]]", " ");
        String[][] hints = {
                {"脱敏", "desensitize"}, {"日志", "log"}, {"安全", "security"}, {"审计", "audit"},
                {"巡检", "inspection"}, {"数据库", "database"}, {"告警", "alert"}, {"监控", "monitor"},
                {"分析", "analysis"}, {"爬虫", "crawler"}, {"图表", "chart"}, {"报表", "report"},
                {"图像", "image"}, {"语音", "voice"}, {"翻译", "translate"}, {"邮件", "email"},
                {"文档", "document"}, {"接口", "api"}, {"测试", "test"}, {"推荐", "recommend"}
        };
        StringBuilder en = new StringBuilder();
        for (String[] h : hints) if (t.contains(h[0])) { en.append(h[1]).append(' '); t = t.replace(h[0], " "); }
        String kw = (t.trim() + " " + en.toString().trim()).trim().replaceAll("\\s+", " ");
        return kw.isEmpty() ? text.trim() : kw;
    }

    private void removeLastMessage() {
        if (messageList.getComponentCount() > 0) {
            messageList.remove(messageList.getComponentCount() - 1);
            messageList.revalidate();
            messageList.repaint();
        }
    }

    private void render(String role, String text, boolean scroll) {
        boolean mine = "me".equals(role);
        JPanel row = new JPanel(new BorderLayout(8, 0));
        row.setOpaque(false);
        row.setBorder(new EmptyBorder(5, 12, 5, 12));
        row.setAlignmentX(Component.LEFT_ALIGNMENT);

        JLabel avatar = new JLabel(new AvatarIcon(mine ? "我" : (active != null ? active.avatarText : "AI"),
                mine ? new Color(0x576B95) : (active != null ? active.color : BRAND), 38));
        Bubble bubble = new Bubble(text, mine ? MY_BUBBLE : OTHER_BUBBLE);

        JPanel holder = new JPanel(new FlowLayout(mine ? FlowLayout.RIGHT : FlowLayout.LEFT, 0, 0));
        holder.setOpaque(false);
        holder.add(bubble);

        if (mine) {
            row.add(holder, BorderLayout.CENTER);
            row.add(avatar, BorderLayout.EAST);
        } else {
            row.add(avatar, BorderLayout.WEST);
            row.add(holder, BorderLayout.CENTER);
        }
        messageList.add(row);
        messageList.revalidate();
        if (scroll) SwingUtilities.invokeLater(() -> {
            JScrollBar bar = messageScroll.getVerticalScrollBar();
            bar.setValue(bar.getMaximum());
        });
    }

    // ---------------- 圆角气泡 ----------------
    private static class Bubble extends JPanel {
        private final Color bg;
        Bubble(String text, Color bg) {
            this.bg = bg;
            setOpaque(false);
            setLayout(new BorderLayout());
            setBorder(new EmptyBorder(9, 13, 9, 13));
            JTextArea area = new JTextArea(text);
            area.setEditable(false);
            area.setLineWrap(true);
            area.setWrapStyleWord(true);
            area.setOpaque(false);
            area.setForeground(new Color(0x1A1A1A));
            area.setFont(new Font("Microsoft YaHei", Font.PLAIN, 14));
            area.setColumns(24);
            add(area, BorderLayout.CENTER);
        }
        @Override protected void paintComponent(Graphics g) {
            Graphics2D g2 = (Graphics2D) g.create();
            g2.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
            g2.setColor(new Color(0, 0, 0, 18));
            g2.fillRoundRect(0, 1, getWidth(), getHeight(), 14, 14);
            g2.setColor(bg);
            g2.fillRoundRect(0, 0, getWidth(), getHeight() - 1, 14, 14);
            g2.dispose();
            super.paintComponent(g);
        }
    }

    // ---------------- 自绘绿色圆角按钮 (Windows L&F 会忽略 setBackground) ----------------
    private static class RoundButton extends JButton {
        private final Color bg;
        RoundButton(String text, Color bg) {
            super(text);
            this.bg = bg;
            setContentAreaFilled(false);
            setBorderPainted(false);
            setFocusPainted(false);
            setForeground(Color.WHITE);
            setFont(getFont().deriveFont(Font.BOLD, 13f));
        }
        @Override protected void paintComponent(Graphics g) {
            Graphics2D g2 = (Graphics2D) g.create();
            g2.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
            g2.setColor(getModel().isPressed() ? bg.darker() : bg);
            g2.fillRoundRect(0, 0, getWidth(), getHeight(), 10, 10);
            g2.dispose();
            super.paintComponent(g);
        }
    }

    // ---------------- 圆形头像 ----------------
    static class AvatarIcon implements Icon {
        private final String text;
        private final Color color;
        private final int size;
        AvatarIcon(String text, Color color, int size) { this.text = text; this.color = color; this.size = size; }
        @Override public int getIconWidth() { return size; }
        @Override public int getIconHeight() { return size; }
        @Override public void paintIcon(Component c, Graphics g, int x, int y) {
            Graphics2D g2 = (Graphics2D) g.create();
            g2.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
            g2.setColor(color);
            g2.fillOval(x, y, size, size);
            g2.setColor(Color.WHITE);
            g2.setFont(new Font("Microsoft YaHei", Font.BOLD, Math.max(11, size / 2)));
            FontMetrics fm = g2.getFontMetrics();
            String t = text == null ? "?" : text;
            int tx = x + (size - fm.stringWidth(t)) / 2;
            int ty = y + (size - fm.getHeight()) / 2 + fm.getAscent();
            g2.drawString(t, tx, ty);
            g2.dispose();
        }
    }
}
