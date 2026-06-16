package web

const indexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Kai Systemctl</title>
  <style>
    :root { color-scheme: light dark; font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; background: #f6f7f9; color: #1f2328; }
    header { padding: 20px 28px; background: #111827; color: white; display: flex; align-items: center; justify-content: space-between; }
    header h1 { margin: 0; font-size: 20px; letter-spacing: 0; }
    main { display: grid; grid-template-columns: 360px 1fr; gap: 20px; padding: 20px; }
    section { background: white; border: 1px solid #d8dee4; border-radius: 8px; overflow: hidden; }
    h2 { margin: 0; padding: 14px 16px; font-size: 14px; border-bottom: 1px solid #d8dee4; background: #f9fafb; }
    .toolbar, form, .editor { padding: 14px 16px; }
    button { border: 1px solid #c9d1d9; background: #f6f8fa; color: #24292f; border-radius: 6px; padding: 7px 10px; cursor: pointer; }
    button.primary { background: #0969da; color: white; border-color: #0969da; }
    button.danger { color: #cf222e; }
    input, textarea { width: 100%; box-sizing: border-box; border: 1px solid #c9d1d9; border-radius: 6px; padding: 9px 10px; font: inherit; margin: 6px 0 10px; }
    textarea { min-height: 420px; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 13px; line-height: 1.45; }
    label { display: block; font-size: 12px; color: #57606a; font-weight: 600; }
    ul { list-style: none; margin: 0; padding: 0; }
    li { padding: 12px 16px; border-bottom: 1px solid #d8dee4; cursor: pointer; }
    li.active { background: #ddf4ff; }
    li strong { display: block; font-size: 14px; }
    li span { color: #57606a; font-size: 12px; }
    .actions { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 10px; }
    .message { padding: 10px 16px; color: #57606a; min-height: 20px; }
    @media (max-width: 900px) { main { grid-template-columns: 1fr; } textarea { min-height: 300px; } }
  </style>
</head>
<body>
  <header>
    <h1>Kai Systemctl</h1>
    <button onclick="loadUnits()">刷新</button>
  </header>
  <main>
    <section>
      <h2>服务列表</h2>
      <ul id="units"></ul>
      <div class="message" id="listMessage"></div>
    </section>
    <section>
      <h2>创建 / 编辑</h2>
      <form id="createForm">
        <label>名称</label>
        <input id="name" placeholder="demo">
        <label>描述</label>
        <input id="description" placeholder="Demo service">
        <label>ExecStart</label>
        <input id="execStart" placeholder="/usr/bin/example --flag">
        <label>工作目录</label>
        <input id="workingDir" placeholder="/opt/example">
        <label>运行用户</label>
        <input id="user" placeholder="root">
        <button class="primary" type="submit">新建</button>
      </form>
      <div class="editor">
        <label>Unit 内容</label>
        <textarea id="content" spellcheck="false"></textarea>
        <div class="actions">
          <button class="primary" onclick="saveContent()">保存</button>
          <button onclick="renameUnit()">重命名</button>
          <button onclick="action('start')">启动</button>
          <button onclick="action('stop')">停止</button>
          <button onclick="action('restart')">重启</button>
          <button onclick="action('enable')">启用</button>
          <button onclick="action('disable')">禁用</button>
          <button class="danger" onclick="deleteUnit()">删除</button>
        </div>
        <div class="message" id="message"></div>
      </div>
    </section>
  </main>
  <script>
    let selected = "";
    const api = async (url, options = {}) => {
      const res = await fetch(url, { headers: { "Content-Type": "application/json" }, ...options });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || res.statusText);
      return data;
    };
    const setMessage = text => document.getElementById("message").textContent = text;
    async function loadUnits() {
      const list = document.getElementById("units");
      list.innerHTML = "";
      const units = await api("/api/units").catch(err => { document.getElementById("listMessage").textContent = err.message; return []; });
      document.getElementById("listMessage").textContent = units.length ? "" : "暂无 kai 管理的服务";
      units.forEach(unit => {
        const li = document.createElement("li");
        li.className = unit.name === selected ? "active" : "";
        li.innerHTML = "<strong>" + unit.name + "</strong><span>" + (unit.activeState || "unknown") + " · " + (unit.description || "") + "</span>";
        li.onclick = () => selectUnit(unit.name);
        list.appendChild(li);
      });
    }
    async function selectUnit(name) {
      selected = name;
      const data = await api("/api/units/" + encodeURIComponent(name));
      document.getElementById("content").value = data.content;
      setMessage(name);
      loadUnits();
    }
    document.getElementById("createForm").onsubmit = async event => {
      event.preventDefault();
      const payload = {
        name: document.getElementById("name").value,
        description: document.getElementById("description").value,
        execStart: document.getElementById("execStart").value,
        workingDir: document.getElementById("workingDir").value,
        user: document.getElementById("user").value
      };
      const unit = await api("/api/units", { method: "POST", body: JSON.stringify(payload) }).catch(err => setMessage(err.message));
      if (unit && unit.name) { selected = unit.name; setMessage("已创建 " + unit.name); loadUnits(); selectUnit(unit.name); }
    };
    async function saveContent() {
      if (!selected) return setMessage("请选择服务");
      await api("/api/units/" + encodeURIComponent(selected), { method: "PUT", body: JSON.stringify({ content: document.getElementById("content").value }) }).then(() => setMessage("已保存")).catch(err => setMessage(err.message));
      loadUnits();
    }
    async function renameUnit() {
      if (!selected) return setMessage("请选择服务");
      const newName = prompt("新名称", selected.replace(/^kai-/, "").replace(/\.service$/, ""));
      if (!newName) return;
      const unit = await api("/api/units/" + encodeURIComponent(selected), { method: "PUT", body: JSON.stringify({ newName }) }).catch(err => setMessage(err.message));
      if (unit && unit.name) { selected = unit.name; setMessage("已重命名"); loadUnits(); }
    }
    async function deleteUnit() {
      if (!selected || !confirm("删除 " + selected + " ?")) return;
      await api("/api/units/" + encodeURIComponent(selected), { method: "DELETE" }).then(() => { selected = ""; document.getElementById("content").value = ""; setMessage("已删除"); }).catch(err => setMessage(err.message));
      loadUnits();
    }
    async function action(name) {
      if (!selected) return setMessage("请选择服务");
      await api("/api/units/" + encodeURIComponent(selected) + "/action", { method: "POST", body: JSON.stringify({ action: name }) }).then(() => setMessage("已执行 " + name)).catch(err => setMessage(err.message));
      loadUnits();
    }
    loadUnits();
  </script>
</body>
</html>`
