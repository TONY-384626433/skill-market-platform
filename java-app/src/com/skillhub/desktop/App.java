package com.skillhub.desktop;

import javax.swing.*;
import javax.swing.table.DefaultTableModel;
import java.awt.*;
import java.awt.event.MouseAdapter;
import java.awt.event.MouseEvent;
import java.net.URI;
import java.util.List;
import java.util.Map;

/**
 * SkillHub 桌面客户端 (Java / Swing 原生实现)
 * — 连接 SkillHub 后端，检索「企业审核库」与「GitHub 开源」技能。
 */
public class App extends JFrame {

    private final JTextField baseField = new JTextField("http://localhost:8080/api/v1", 34);
    private final JComboBox<String> sourceBox = new JComboBox<>(new String[]{"GitHub 开源", "企业审核库"});
    private final JTextField queryField = new JTextField(24);
    private final JButton connectBtn = new JButton("连接");
    private final JButton searchBtn = new JButton("搜索");
    private final JLabel statusLabel = new JLabel("未连接");
    private final DefaultTableModel model = new DefaultTableModel(new Object[]{"名称"}, 0) {
        @Override public boolean isCellEditable(int r, int c) { return false; }
    };
    private final JTable table = new JTable(model);
    private final SkillHubClient client = new SkillHubClient(baseField.getText());

    public App() {
        super("SkillHub 桌面客户端 · Java Edition");
        setDefaultCloseOperation(EXIT_ON_CLOSE);
        setSize(1100, 680);
        setLocationRelativeTo(null);
        setLayout(new BorderLayout());

        add(buildHeader(), BorderLayout.NORTH);

        table.setRowHeight(26);
        table.setAutoResizeMode(JTable.AUTO_RESIZE_ALL_COLUMNS);
        table.getTableHeader().setReorderingAllowed(false);
        table.addMouseListener(new MouseAdapter() {
            @Override public void mouseClicked(MouseEvent e) {
                if (e.getClickCount() == 2) openSelected();
            }
        });
        add(new JScrollPane(table), BorderLayout.CENTER);

        JPanel south = new JPanel(new BorderLayout());
        south.setBorder(BorderFactory.createEmptyBorder(6, 12, 8, 12));
        statusLabel.setForeground(new Color(0x33, 0x66, 0x99));
        south.add(statusLabel, BorderLayout.WEST);
        add(south, BorderLayout.SOUTH);

        connectBtn.addActionListener(e -> doConnect());
        searchBtn.addActionListener(e -> doSearch());
        queryField.addActionListener(e -> doSearch());

        setStatus("就绪 · 默认后端 " + client.getBase());
    }

    private JPanel buildHeader() {
        JPanel root = new JPanel();
        root.setLayout(new BoxLayout(root, BoxLayout.Y_AXIS));
        root.setBorder(BorderFactory.createEmptyBorder(10, 12, 6, 12));

        JPanel row1 = new JPanel(new FlowLayout(FlowLayout.LEFT, 8, 4));
        row1.add(new JLabel("后端地址"));
        row1.add(baseField);
        row1.add(connectBtn);
        root.add(row1);

        JPanel row2 = new JPanel(new FlowLayout(FlowLayout.LEFT, 8, 4));
        row2.add(new JLabel("来源"));
        row2.add(sourceBox);
        row2.add(new JLabel("关键词"));
        row2.add(queryField);
        row2.add(searchBtn);
        root.add(row2);

        JLabel brand = new JLabel("SkillHub · AI 能力中心 (Java 客户端)");
        brand.setFont(brand.getFont().deriveFont(Font.BOLD, 15f));
        brand.setBorder(BorderFactory.createEmptyBorder(2, 4, 4, 0));
        root.add(brand);

        return root;
    }

    private void setStatus(String text) { statusLabel.setText(text); }

    private void doConnect() {
        final String base = baseField.getText().trim();
        connectBtn.setEnabled(false);
        setStatus("连接中… " + base);
        new SwingWorker<Map<String, Object>, Void>() {
            @Override protected Map<String, Object> doInBackground() throws Exception {
                client.setBase(base);
                return client.health();
            }
            @Override protected void done() {
                connectBtn.setEnabled(true);
                try {
                    Map<String, Object> h = get();
                    setStatus("✅ 已连接 · " + Json.str(h.get("service")) + " v" + Json.str(h.get("version")) + " · " + client.getBase());
                } catch (Exception ex) {
                    setStatus("❌ 连接失败: " + rootMessage(ex));
                }
            }
        }.execute();
    }

    private void doSearch() {
        final String query = queryField.getText().trim();
        final boolean github = sourceBox.getSelectedIndex() == 0;
        searchBtn.setEnabled(false);
        setStatus("检索中… (" + (github ? "GitHub 开源" : "企业审核库") + ") 关键词=" + (query.isEmpty() ? "(空)" : query));
        new SwingWorker<Map<String, Object>, Void>() {
            @Override protected Map<String, Object> doInBackground() throws Exception {
                return github ? client.searchGitHub(query, 1, 30) : client.searchInternal(query, 1, 30);
            }
            @Override protected void done() {
                searchBtn.setEnabled(true);
                try {
                    Map<String, Object> res = get();
                    List<Object> data = Json.asArray(res.get("data"));
                    if (github) fillGitHub(data, res); else fillInternal(data, res);
                } catch (Exception ex) {
                    setStatus("❌ 检索失败: " + rootMessage(ex));
                }
            }
        }.execute();
    }

    private void fillGitHub(List<Object> data, Map<String, Object> res) {
        model.setColumnIdentifiers(new Object[]{"名称", "仓库", "Stars", "许可", "兼容性"});
        model.setRowCount(0);
        for (Object o : data) {
            Map<String, Object> m = Json.asObject(o);
            Map<String, Object> compat = Json.asObject(m.get("compatibility"));
            model.addRow(new Object[]{
                    Json.str(m.get("name")),
                    Json.str(m.get("repository")),
                    Json.str(m.get("stars")),
                    Json.str(m.get("license")),
                    Json.str(compat.get("label")),
            });
        }
        setStatus("✅ GitHub 匹配 " + Json.str(res.get("total")) + " 项 · 本页 " + data.size()
                + " · 账号连接=" + Json.str(res.get("authenticated")));
    }

    private void fillInternal(List<Object> data, Map<String, Object> res) {
        model.setColumnIdentifiers(new Object[]{"名称", "分类", "版本", "安装量", "评分"});
        model.setRowCount(0);
        for (Object o : data) {
            Map<String, Object> m = Json.asObject(o);
            model.addRow(new Object[]{
                    Json.str(m.get("name")),
                    Json.str(m.get("category")),
                    Json.str(m.get("version")),
                    Json.str(m.get("install_count")),
                    Json.str(m.get("rating")),
            });
        }
        setStatus("✅ 企业库匹配 " + Json.str(res.get("total")) + " 项 · 本页 " + data.size());
    }

    private void openSelected() {
        int row = table.getSelectedRow();
        if (row < 0) return;
        int repoCol = -1;
        for (int c = 0; c < model.getColumnCount(); c++) {
            if ("仓库".equals(model.getColumnName(c))) { repoCol = c; break; }
        }
        String repo = repoCol >= 0 ? Json.str(model.getValueAt(row, repoCol)) : "";
        String name = Json.str(model.getValueAt(row, 0));
        if (repo.isEmpty()) { setStatus("· " + name + " (双击行仅在 GitHub 来源下可跳转)"); return; }
        try {
            Desktop.getDesktop().browse(new URI("https://github.com/" + repo));
            setStatus("↗ 已在浏览器打开 " + repo);
        } catch (Exception ex) {
            setStatus("打开浏览器失败: " + ex.getMessage());
        }
    }

    private static String rootMessage(Throwable t) {
        Throwable c = t;
        while (c.getCause() != null) c = c.getCause();
        return c.getMessage() == null ? c.toString() : c.getMessage();
    }

    public static void main(String[] args) {
        try { UIManager.setLookAndFeel(UIManager.getSystemLookAndFeelClassName()); } catch (Exception ignored) { }
        SwingUtilities.invokeLater(() -> new App().setVisible(true));
    }
}
