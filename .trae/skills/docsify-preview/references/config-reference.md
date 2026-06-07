# Docsify 配置与资源参考

本文件包含 Docsify 站点的所有配置规范和资源映射关系。当技能执行到配置修复或资源下载步骤时，按需读取本文件。

---

## CDN → 本地路径替换映射

当 `index.html` 中存在 CDN 远程引用时，按以下映射替换为本地文件：

| CDN 引用 | 本地替代 |
|---------|---------|
| `//cdn.jsdelivr.net/npm/docsify@4/lib/themes/vue.css` | `assets/docsify-vue.css` |
| `//cdn.jsdelivr.net/npm/docsify@4` | `assets/docsify.min.js` |
| `//cdn.jsdelivr.net/npm/docsify/lib/plugins/search.min.js` | `assets/docsify-search.min.js` |
| `//cdn.jsdelivr.net/npm/docsify/lib/plugins/emoji.min.js` | `assets/docsify-emoji.min.js` |
| `//cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.min.js` | `assets/mermaid.min.js` |
| `//cdn.jsdelivr.net/npm/js-yaml@4/dist/js-yaml.min.js` | `assets/js-yaml.min.js` |

---

## Docsify 必需配置项

`window.$docsify` 中必须包含以下配置。缺失则补充，不删除用户已有的自定义配置（如 Mermaid 插件、YAML 卡片等）。

站点名称 `name` 字段的值应与 Step 1 中解析的站点名称一致，不要硬编码：

```javascript
window.$docsify = {
  name: "<Step 1 解析的站点名称>",
  loadSidebar: "_sidebar.md",
  loadNavbar: "_navbar.md",
  subMaxLevel: 3,
  auto2top: true,
  relativePath: true,
  search: { placeholder: "搜索文档", noData: "没有结果", depth: 6 }
}
```

---

## 资源下载清单

| 文件名 | 下载地址 |
|-------|---------|
| docsify-vue.css | `https://cdn.jsdelivr.net/npm/docsify@4/lib/themes/vue.css` |
| docsify.min.js | `https://cdn.jsdelivr.net/npm/docsify@4` |
| docsify-search.min.js | `https://cdn.jsdelivr.net/npm/docsify/lib/plugins/search.min.js` |
| docsify-emoji.min.js | `https://cdn.jsdelivr.net/npm/docsify/lib/plugins/emoji.min.js` |
| mermaid.min.js | `https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.min.js` |
| js-yaml.min.js | `https://cdn.jsdelivr.net/npm/js-yaml@4/dist/js-yaml.min.js` |

下载命令模板（仅下载快照中标记缺失/损坏的文件）：

```bash
cd <项目根目录>/docs/assets
# 仅对快照中标记 ❌缺失 或 ⚠️损坏 的文件执行：
curl -sL "<下载地址>" -o <文件名>
```

下载后验证文件大小 > 1KB，防止空文件。下载失败则重试一次。
