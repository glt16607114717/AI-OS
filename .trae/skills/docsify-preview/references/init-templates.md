# 站点初始化模板

本文件包含 Docsify 站点从零初始化时所需的 `index.html` 模板。仅在 `docs/index.html` 不存在时读取并创建。

`_sidebar.md` 和 `_navbar.md` 不在此处提供模板，而是由技能 Step 3 根据项目实际目录结构动态生成。

---

## 站点名称解析规则

模板中所有 `{SITE_NAME}` 占位符需在创建时替换为实际站点名称。解析规则与 SKILL.md Step 1.2 完全一致，此处不重复定义——以 SKILL.md 为唯一权威来源。

---

## docs/index.html

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <title>{SITE_NAME}</title>
  <meta http-equiv="X-UA-Compatible" content="IE=edge,chrome=1" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0, minimum-scale=1.0">
  <link rel="stylesheet" href="assets/docsify-vue.css">
  <style>
    .content {
      max-width: unset !important;
      width: calc(100% - 300px);
      left: 300px;
    }
    .markdown-section {
      max-width: unset !important;
      margin: 0 !important;
      padding: 30px 40px 40px !important;
    }

    .mermaid-wrapper {
      margin: 1.5em 0;
      padding: 20px 0;
      background-color: #fafbfc;
      border-radius: 8px;
      border: 1px solid #ebedf0;
      overflow-x: auto;
      display: flex;
      justify-content: center;
    }
    .mermaid-wrapper svg {
      height: auto;
    }
    .mermaid-gantt svg {
      min-width: max-content;
    }

    .doc-meta-card {
      background: linear-gradient(135deg, #f8fafc 0%, #f1f5f9 100%);
      border: 1px solid #e2e8f0;
      border-left: 4px solid #2563eb;
      border-radius: 8px;
      padding: 16px 20px;
      margin-bottom: 24px;
      display: flex;
      flex-wrap: wrap;
      gap: 12px 20px;
      font-size: 0.9em;
      line-height: 1.6;
    }
    .meta-item {
      display: flex;
      align-items: center;
      gap: 6px;
    }
    .meta-label {
      color: #94a3b8;
      font-weight: 600;
      font-size: 0.85em;
      white-space: nowrap;
    }
    .meta-value {
      color: #334155;
      font-weight: 500;
    }
    .meta-value a {
      color: #3b82f6;
      text-decoration: none;
    }
    .meta-value a:hover { text-decoration: underline; }

    .meta-value.badge {
      padding: 2px 10px;
      border-radius: 12px;
      font-size: 0.85em;
      font-weight: 600;
      display: inline-block;
    }
    .badge-status-default { background: #f1f5f9; color: #64748b; border: 1px solid #cbd5e1; }
    .badge-status-done, .badge-status-completed, .badge-status-已完成 { background: #f0fdf4; color: #16a34a; border: 1px solid #86efac; }
    .badge-status-active, .badge-status-in-progress, .badge-status-进行中, .badge-status-打样中 { background: #eff6ff; color: #2563eb; border: 1px solid #93c5fd; }
    .badge-status-crit, .badge-status-紧急, .badge-status-延期 { background: #fef2f2; color: #dc2626; border: 1px solid #fca5a5; }
    .badge-status-review, .badge-status-评审中 { background: #fffbeb; color: #d97706; border: 1px solid #fcd34d; }
    .badge-status-planning, .badge-status-计划中 { background: #faf5ff; color: #9333ea; border: 1px solid #d8b4fe; }

    .meta-separator {
      width: 100%;
      height: 0;
      display: none;
    }
  </style>
</head>
<body>
  <div id="app"></div>

  <script src="assets/js-yaml.min.js"></script>
  <script src="assets/mermaid.min.js"></script>

  <script>
    mermaid.initialize({
      startOnLoad: false,
      useMaxWidth: true,
      theme: 'base',
      themeVariables: {
        fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
        fontSize: '12px',
        primaryColor: '#e0e7ff',
        primaryBorderColor: '#4f46e5',
        primaryTextColor: '#1e3a8a',
        secondaryColor: '#dcfce7',
        tertiaryColor: '#ffedd5',
        lineColor: '#2563eb',
        textColor: '#0f172a',
        clusterBkg: '#f8fafc',
        clusterBorder: '#94a3b8',
        actorBkg: '#ffffff',
        actorBorder: '#3b82f6',
        signalColor: '#e11d48',
        signalTextColor: '#9f1239'
      },
      flowchart: {
        htmlLabels: true,
        curve: 'basis',
        nodeSpacing: 20,
        rankSpacing: 25,
        padding: 10
      },
      sequence: {
        mirrorActors: false,
        bottomMarginAdj: 10,
        actorFontSize: 13,
        noteFontSize: 12,
        messageFontSize: 12,
        messageMargin: 35
      },
      gantt: {
        useWidth: 1200,
        barHeight: 22,
        barGap: 4,
        fontSize: 12,
        topPadding: 50,
        leftPadding: 75,
        gridLineStartPadding: 35,
        numberSectionStyles: 4,
        sectionBkgColor: 'rgba(241, 245, 249, 0.6)'
      }
    });

    var mermaidRenderCount = 0;
    window.$docsify = {
      name: "{SITE_NAME}",
      loadSidebar: "_sidebar.md",
      loadNavbar: "_navbar.md",
      subMaxLevel: 3,
      auto2top: true,
      relativePath: true,
      search: { placeholder: "搜索文档", noData: "没有结果", depth: 6 },

      markdown: {
        renderer: {
          code: function(code, lang) {
            if (lang === 'mermaid') {
              var id = 'mermaid-' + mermaidRenderCount++;
              var firstLine = code.trim().split('\n')[0].toLowerCase();
              var type = firstLine.indexOf('gantt') === 0 ? 'gantt' : '';
              return '<div class="mermaid-wrapper' + (type ? ' mermaid-' + type : '') + '"><pre id="' + id + '">' + code + '</pre></div>';
            }
            return this.origin.code.apply(this, arguments);
          }
        }
      },

      plugins: [
        function(hook) {
          hook.beforeEach(function(content) {
            var yamlRegex = /^---\n([\s\S]*?)\n---\n/;
            var match = content.match(yamlRegex);
            if (!match) return content;
            try {
              var meta = jsyaml.load(match[1]);
              var labelMap = {
                title: '标题', author: '作者', email: '邮箱', date: '日期',
                project_code: '项目编号', department: '部门', status: '状态',
                priority: '优先级', completion_rate: '完成率'
              };
              var html = '<div class="doc-meta-card">';
              for (var key in meta) {
                if (!meta.hasOwnProperty(key)) continue;
                var value = meta[key];
                var label = labelMap[key] || key;
                var valStr = String(value);
                if (key.toLowerCase() === 'status' || key === '状态') {
                  var cls = 'badge-status-default';
                  var lc = valStr.toLowerCase();
                  if (/完成|done|completed/.test(lc)) cls = 'badge-status-done';
                  else if (/进行中|active|in.progress|打样/.test(lc)) cls = 'badge-status-active';
                  else if (/紧急|crit|延期/.test(lc)) cls = 'badge-status-crit';
                  else if (/评审|review/.test(lc)) cls = 'badge-status-review';
                  else if (/计划|planning/.test(lc)) cls = 'badge-status-planning';
                  html += '<div class="meta-item"><span class="meta-label">' + label + '</span><span class="meta-value badge ' + cls + '">' + valStr + '</span></div>';
                } else if (key.toLowerCase().indexOf('link') !== -1 && valStr.indexOf('http') === 0) {
                  html += '<div class="meta-item"><span class="meta-label">' + label + '</span><span class="meta-value"><a href="' + valStr + '" target="_blank">🔗 查看链接</a></span></div>';
                } else if (key.toLowerCase() === 'email') {
                  html += '<div class="meta-item"><span class="meta-label">' + label + '</span><span class="meta-value"><a href="mailto:' + valStr + '">' + valStr + '</a></span></div>';
                } else if (key.toLowerCase() === 'priority') {
                  var pVal = valStr.replace(/^(P|p)/, 'P');
                  html += '<div class="meta-item"><span class="meta-label">' + label + '</span><span class="meta-value">' + pVal + '</span></div>';
                } else {
                  html += '<div class="meta-item"><span class="meta-label">' + label + '</span><span class="meta-value">' + valStr + '</span></div>';
                }
              }
              html += '</div>\n\n';
              return content.replace(yamlRegex, html);
            } catch (e) {
              console.error('YAML 解析失败:', e);
              return content;
            }
          });

          hook.afterEach(function(html) {
            setTimeout(function() {
              var elements = document.querySelectorAll('.mermaid-wrapper pre');
              if (elements.length > 0) {
                elements.forEach(function(el) {
                  var id = el.id;
                  mermaid.render(id + '-svg', el.textContent).then(function(result) {
                    el.parentElement.innerHTML = result.svg;
                  }).catch(function(e) {
                    el.parentElement.innerHTML = '<p style="color:red">Mermaid 渲染错误: ' + e.message + '</p>';
                  });
                });
              }
            }, 300);
          });
        }
      ]
    }
  </script>

  <script src="assets/docsify.min.js"></script>
  <script src="assets/docsify-search.min.js"></script>
  <script src="assets/docsify-emoji.min.js"></script>
</body>
</html>
```
