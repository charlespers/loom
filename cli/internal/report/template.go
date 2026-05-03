package report

// reportTemplate is the single HTML/CSS/JS file Loom emits per run.
// Self-contained: no external assets, no network requests at load time.
// Embedded data sits in two <script type="application/json"> blocks so an
// auditor can re-derive every number we display, and a vanilla-JS chain
// verifier re-hashes audit records using the SubtleCrypto API.
//
// Design language follows the spec's "showroom" register: restrained
// color (one accent per category), typographic hierarchy, generous
// whitespace, no decorative chrome. Print styles strip interactive
// controls so the same file becomes a clean PDF.
const reportTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="generator" content="loom v{{.LoomVersion}}">
  <title>loom · run {{.RunID}}</title>
  <style>
    :root {
      --bg:        #faf9f5;
      --surface:   #ffffff;
      --ink:       #0f172a;
      --ink-soft:  #475569;
      --ink-mute:  #94a3b8;
      --rule:      #e6e3da;
      --rule-soft: #f1eee5;
      --accent:    #1e293b;
      --c-span:      #0d9488;
      --c-metric:    #4f46e5;
      --c-audit:     #b45309;
      --c-lifecycle: #64748b;
      --c-error:     #dc2626;
      --c-ok:        #15803d;
      --c-warn:      #b45309;
      --shadow:    0 1px 0 rgba(15,23,42,.04), 0 8px 32px -16px rgba(15,23,42,.08);
    }

    * { box-sizing: border-box; }
    html, body { margin: 0; padding: 0; }
    body {
      background: var(--bg);
      color: var(--ink);
      font: 15px/1.55 ui-sans-serif, -apple-system, "SF Pro Text", "Segoe UI",
            Roboto, "Helvetica Neue", Arial, sans-serif;
      -webkit-font-smoothing: antialiased;
      text-rendering: optimizeLegibility;
    }
    code, pre, .mono {
      font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas,
                   "Liberation Mono", monospace;
      font-feature-settings: "tnum" on, "ss01" on;
      font-size: 0.92em;
    }

    .page {
      max-width: 1080px;
      margin: 0 auto;
      padding: 56px 32px 96px;
    }

    /* ── Cover ─────────────────────────────────────────────────────── */
    header.cover { margin-bottom: 48px; }
    .eyebrow {
      font-size: 11px;
      letter-spacing: 0.16em;
      text-transform: uppercase;
      color: var(--ink-mute);
      margin-bottom: 14px;
    }
    .cover h1 {
      font-size: 28px;
      font-weight: 600;
      letter-spacing: -0.01em;
      margin: 0 0 16px;
      line-height: 1.15;
      display: flex;
      align-items: baseline;
      gap: 14px;
      flex-wrap: wrap;
    }
    .cover h1 .id {
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-size: 22px;
      color: var(--ink-soft);
    }
    .badge {
      display: inline-block;
      padding: 4px 10px;
      font-size: 11px;
      font-weight: 600;
      letter-spacing: 0.06em;
      text-transform: uppercase;
      border-radius: 4px;
      vertical-align: middle;
    }
    .badge.ok    { color: var(--c-ok);    background: rgba(21,128,61,.08); }
    .badge.warn  { color: var(--c-warn);  background: rgba(180,83,9,.08); }
    .badge.error { color: var(--c-error); background: rgba(220,38,38,.08); }

    .meta {
      display: grid;
      grid-template-columns: max-content 1fr;
      column-gap: 24px;
      row-gap: 8px;
      margin-top: 8px;
    }
    .meta dt {
      font-size: 12px;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--ink-mute);
      align-self: center;
    }
    .meta dd {
      margin: 0;
      color: var(--ink);
      font-variant-numeric: tabular-nums;
    }
    .meta dd code { background: var(--rule-soft); padding: 1px 6px; border-radius: 3px; }

    /* ── At-a-glance cards ─────────────────────────────────────────── */
    .glance {
      display: grid;
      grid-template-columns: repeat(5, 1fr);
      gap: 12px;
      margin: 32px 0 48px;
    }
    .glance .card {
      background: var(--surface);
      border: 1px solid var(--rule);
      border-radius: 8px;
      padding: 14px 16px;
    }
    .glance .card .label {
      font-size: 11px;
      letter-spacing: 0.1em;
      text-transform: uppercase;
      color: var(--ink-mute);
      margin-bottom: 6px;
    }
    .glance .card .value {
      font-family: ui-monospace, monospace;
      font-size: 22px;
      font-weight: 600;
      letter-spacing: -0.01em;
    }
    .glance .card.span      .value { color: var(--c-span); }
    .glance .card.metric    .value { color: var(--c-metric); }
    .glance .card.audit     .value { color: var(--c-audit); }
    .glance .card.lifecycle .value { color: var(--c-lifecycle); }
    .glance .card.error     .value { color: var(--c-error); }

    /* ── Sections ──────────────────────────────────────────────────── */
    section { margin: 0 0 48px; }
    section > h2 {
      font-size: 16px;
      font-weight: 600;
      letter-spacing: -0.005em;
      margin: 0 0 4px;
      display: flex;
      align-items: baseline;
      gap: 10px;
    }
    section > h2 .count {
      font-size: 12px;
      font-weight: 400;
      color: var(--ink-mute);
      font-variant-numeric: tabular-nums;
    }
    section > .sub {
      font-size: 13px;
      color: var(--ink-soft);
      margin: 0 0 18px;
    }

    /* ── Lifecycle outline ─────────────────────────────────────────── */
    .lifecycle ol {
      list-style: none;
      padding: 0;
      margin: 0;
      border-left: 1px solid var(--rule);
    }
    .lifecycle li {
      display: grid;
      grid-template-columns: 110px 1fr;
      column-gap: 16px;
      padding: 6px 0 6px 18px;
      position: relative;
    }
    .lifecycle li::before {
      content: "";
      position: absolute;
      left: -4px; top: 14px;
      width: 7px; height: 7px;
      border-radius: 50%;
      background: var(--c-lifecycle);
    }
    .lifecycle .t { color: var(--ink-mute); font-size: 12px; }
    .lifecycle .m { color: var(--ink); }

    /* ── Tables ────────────────────────────────────────────────────── */
    table {
      width: 100%;
      border-collapse: separate;
      border-spacing: 0;
      font-variant-numeric: tabular-nums;
    }
    th, td {
      text-align: left;
      padding: 8px 12px;
      border-bottom: 1px solid var(--rule-soft);
      font-size: 13.5px;
    }
    th {
      font-weight: 600;
      font-size: 11px;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--ink-mute);
      border-bottom: 1px solid var(--rule);
    }
    td.num, th.num { text-align: right; }
    tbody tr:hover { background: var(--rule-soft); }

    /* Span row bar */
    .barcell {
      width: 140px;
      padding-right: 0;
    }
    .bar {
      height: 6px;
      background: var(--rule-soft);
      border-radius: 3px;
      overflow: hidden;
      position: relative;
    }
    .bar > i {
      display: block;
      height: 100%;
      background: var(--c-span);
      border-radius: 3px;
    }

    /* Sparkline */
    .spark { width: 120px; height: 28px; }
    .spark path { fill: none; stroke: var(--c-metric); stroke-width: 1.4; stroke-linejoin: round; stroke-linecap: round; }

    /* ── Audit ─────────────────────────────────────────────────────── */
    .audit-head {
      background: var(--surface);
      border: 1px solid var(--rule);
      border-radius: 8px;
      padding: 12px 16px;
      display: flex;
      align-items: center;
      gap: 16px;
      flex-wrap: wrap;
      margin-bottom: 18px;
    }
    .audit-head .label {
      font-size: 11px;
      letter-spacing: 0.1em;
      text-transform: uppercase;
      color: var(--ink-mute);
    }
    .audit-head .head {
      font-family: ui-monospace, monospace;
      font-size: 12px;
      color: var(--ink);
      flex: 1;
      overflow-wrap: anywhere;
    }
    button.verify {
      font: 600 12px/1 ui-sans-serif, system-ui, sans-serif;
      letter-spacing: 0.06em;
      text-transform: uppercase;
      padding: 8px 14px;
      border-radius: 6px;
      border: 1px solid var(--rule);
      background: var(--surface);
      color: var(--ink);
      cursor: pointer;
    }
    button.verify:hover { background: var(--rule-soft); }
    .verify-status {
      font-size: 13px;
      color: var(--ink-soft);
      margin-left: auto;
    }
    .verify-status.ok    { color: var(--c-ok); }
    .verify-status.fail  { color: var(--c-error); }

    /* ── Errors ────────────────────────────────────────────────────── */
    .error-card {
      background: var(--surface);
      border: 1px solid var(--rule);
      border-left: 3px solid var(--c-error);
      border-radius: 6px;
      padding: 12px 16px;
      margin-bottom: 8px;
    }
    .error-card.warn { border-left-color: var(--c-warn); }
    .error-card .head {
      display: flex;
      align-items: baseline;
      gap: 10px;
      margin-bottom: 6px;
    }
    .error-card .head .name { font-weight: 600; font-family: ui-monospace, monospace; }
    .error-card .head .sev {
      font-size: 11px;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--c-error);
    }
    .error-card.warn .head .sev { color: var(--c-warn); }
    .error-card .head .time { color: var(--ink-mute); font-size: 12px; margin-left: auto; }
    .error-card .msg { color: var(--ink-soft); font-size: 13.5px; }
    .error-card .attrs {
      font-family: ui-monospace, monospace;
      font-size: 11.5px;
      color: var(--ink-mute);
      margin-top: 6px;
      overflow-wrap: anywhere;
    }

    /* ── Event stream filter ──────────────────────────────────────── */
    .stream {
      background: var(--surface);
      border: 1px solid var(--rule);
      border-radius: 8px;
      overflow: hidden;
    }
    .stream .controls {
      padding: 12px 16px;
      border-bottom: 1px solid var(--rule);
      display: flex;
      gap: 12px;
      flex-wrap: wrap;
      align-items: center;
    }
    .stream input[type=search] {
      flex: 1;
      min-width: 220px;
      font: 14px ui-sans-serif, system-ui, sans-serif;
      padding: 7px 12px;
      border-radius: 6px;
      border: 1px solid var(--rule);
      background: var(--bg);
    }
    .stream input[type=search]:focus { outline: 2px solid var(--c-metric); outline-offset: 1px; }
    .stream .toggles { display: flex; gap: 4px; flex-wrap: wrap; }
    .stream .toggle {
      font-size: 12px;
      letter-spacing: 0.04em;
      padding: 4px 9px;
      border-radius: 4px;
      border: 1px solid transparent;
      cursor: pointer;
      user-select: none;
      background: var(--rule-soft);
      color: var(--ink-soft);
    }
    .stream .toggle.active.span      { background: rgba(13,148,136,.10);  color: var(--c-span);      border-color: rgba(13,148,136,.18); }
    .stream .toggle.active.metric    { background: rgba(79,70,229,.10);   color: var(--c-metric);    border-color: rgba(79,70,229,.18); }
    .stream .toggle.active.audit     { background: rgba(180,83,9,.10);    color: var(--c-audit);     border-color: rgba(180,83,9,.18); }
    .stream .toggle.active.lifecycle { background: rgba(100,116,139,.10); color: var(--c-lifecycle); border-color: rgba(100,116,139,.18); }
    .stream .toggle.active.error     { background: rgba(220,38,38,.10);   color: var(--c-error);     border-color: rgba(220,38,38,.18); }
    .stream .results { padding: 4px 0; max-height: 480px; overflow: auto; }
    .stream .row {
      display: grid;
      grid-template-columns: 90px 70px 1fr;
      column-gap: 12px;
      padding: 4px 16px;
      font-family: ui-monospace, monospace;
      font-size: 12.5px;
      align-items: baseline;
      border-bottom: 1px solid var(--rule-soft);
    }
    .stream .row:last-child { border-bottom: none; }
    .stream .row .t { color: var(--ink-mute); }
    .stream .row .c {
      font-weight: 600;
      letter-spacing: 0.04em;
      font-size: 11px;
      text-transform: uppercase;
    }
    .stream .row.span      .c { color: var(--c-span); }
    .stream .row.metric    .c { color: var(--c-metric); }
    .stream .row.audit     .c { color: var(--c-audit); }
    .stream .row.lifecycle .c { color: var(--c-lifecycle); }
    .stream .row.error     .c { color: var(--c-error); }
    .stream .row .b { color: var(--ink); overflow-wrap: anywhere; }
    .stream .empty { padding: 24px; text-align: center; color: var(--ink-mute); font-size: 13px; }

    footer.foot {
      margin-top: 64px;
      padding-top: 18px;
      border-top: 1px solid var(--rule);
      font-size: 12px;
      color: var(--ink-mute);
      display: flex;
      justify-content: space-between;
      flex-wrap: wrap;
      gap: 12px;
    }

    /* ── Print ─────────────────────────────────────────────────────── */
    @media print {
      :root { --bg: #fff; --surface: #fff; --shadow: none; }
      body { font-size: 11pt; }
      .page { padding: 0; max-width: none; }
      .stream .results { max-height: none; overflow: visible; }
      .stream input, .stream .toggle, button.verify, .stream .controls { display: none !important; }
      section { break-inside: avoid; }
      header.cover { break-after: avoid; }
    }
  </style>
</head>
<body>
<main class="page">

  <header class="cover">
    <div class="eyebrow">Loom · loom.event.v1</div>
    <h1>
      Run <span class="id">{{.RunID}}</span>
      <span class="badge {{.StatusBadge}}">{{.Status}}</span>
    </h1>
    <dl class="meta">
      <dt>Started</dt>   <dd>{{.StartedHuman}}</dd>
      <dt>Ended</dt>     <dd>{{.EndedHuman}} <span class="mono" style="color:var(--ink-mute)">· {{.Duration}}</span></dd>
      <dt>Command</dt>   <dd><code>{{.Command}}</code></dd>
      <dt>Host</dt>      <dd><code>{{.Hostname}}</code> · {{.OS}}/{{.Arch}} · kernel {{.Kernel}}</dd>
      <dt>PID</dt>       <dd class="mono">{{.PID}}</dd>
    </dl>
  </header>

  <div class="glance">
    <div class="card span">      <div class="label">Spans</div>      <div class="value">{{.Counts.Span}}</div></div>
    <div class="card metric">    <div class="label">Metrics</div>    <div class="value">{{.Counts.Metric}}</div></div>
    <div class="card audit">     <div class="label">Audit</div>      <div class="value">{{.Counts.Audit}}</div></div>
    <div class="card lifecycle"> <div class="label">Lifecycle</div>  <div class="value">{{.Counts.Lifecycle}}</div></div>
    <div class="card error">     <div class="label">Errors</div>     <div class="value">{{.Counts.Error}}</div></div>
  </div>

  {{if .Lifecycles}}
  <section class="lifecycle">
    <h2>Lifecycle <span class="count">{{len .Lifecycles}} markers</span></h2>
    <p class="sub">Sparse named anchors describing the run's structural milestones.</p>
    <ol>
      {{range .Lifecycles}}
        <li id="lc-{{.Anchor}}">
          <span class="t mono">{{.Time}}</span>
          <span class="m mono">{{.Marker}}</span>
        </li>
      {{end}}
    </ol>
  </section>
  {{end}}

  {{if .Spans}}
  <section>
    <h2>Spans <span class="count">by total time</span></h2>
    <p class="sub">Aggregated per name. Bar shows fraction of run wall-clock.</p>
    <table>
      <thead>
        <tr>
          <th>Name</th>
          <th class="barcell">% of run</th>
          <th class="num">Count</th>
          <th class="num">Total</th>
          <th class="num">Mean</th>
          <th class="num">p50</th>
          <th class="num">p95</th>
          <th class="num">p99</th>
          <th class="num">Max</th>
        </tr>
      </thead>
      <tbody>
        {{range .Spans}}
        <tr>
          <td class="mono">{{.Name}}</td>
          <td class="barcell"><div class="bar"><i style="width: {{printf "%.1f" .BarPct}}%"></i></div></td>
          <td class="num mono">{{.Count}}</td>
          <td class="num mono">{{.Total}}</td>
          <td class="num mono">{{.Mean}}</td>
          <td class="num mono">{{.P50}}</td>
          <td class="num mono">{{.P95}}</td>
          <td class="num mono">{{.P99}}</td>
          <td class="num mono">{{.Max}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
  </section>
  {{end}}

  {{if .Metrics}}
  <section>
    <h2>Metrics <span class="count">by name</span></h2>
    <p class="sub">Numeric samples. Sparkline shows the chronological sequence of values.</p>
    <table>
      <thead>
        <tr>
          <th>Name</th>
          <th>Kind</th>
          <th class="num">Count</th>
          <th class="num">Last</th>
          <th class="num">Min</th>
          <th class="num">Max</th>
          <th class="num">Mean</th>
          <th>Trend</th>
        </tr>
      </thead>
      <tbody>
        {{range .Metrics}}
        <tr>
          <td class="mono">{{.Name}}</td>
          <td class="mono" style="color:var(--ink-mute)">{{.Kind}}</td>
          <td class="num mono">{{.Count}}</td>
          <td class="num mono">{{.Last}}</td>
          <td class="num mono">{{.Min}}</td>
          <td class="num mono">{{.Max}}</td>
          <td class="num mono">{{.Mean}}</td>
          <td><svg class="spark" viewBox="0 0 120 28" preserveAspectRatio="none"><path d="{{.Sparkline}}"/></svg></td>
        </tr>
        {{end}}
      </tbody>
    </table>
  </section>
  {{end}}

  <section>
    <h2>Audit <span class="count">{{.AuditCount}} record{{if ne .AuditCount 1}}s{{end}}, hash-chained</span></h2>
    <p class="sub">SHA-256 chain over canonical record bytes. Click <em>verify</em> to re-hash in your browser.</p>
    <div class="audit-head">
      <span class="label">Chain head</span>
      <span class="head mono" id="chain-head">{{.AuditHead}}</span>
      <button class="verify" id="verify-btn" type="button">Verify</button>
      <span class="verify-status" id="verify-status">unverified</span>
    </div>
    {{if .AuditRecords}}
    <table>
      <thead>
        <tr>
          <th>Time</th>
          <th class="num">Seq</th>
          <th>Name</th>
          <th>Prev</th>
          <th>This</th>
        </tr>
      </thead>
      <tbody>
        {{range .AuditRecords}}
        <tr>
          <td class="mono" style="color:var(--ink-mute)">{{.Time}}</td>
          <td class="num mono">{{.Seq}}</td>
          <td class="mono">{{.Name}}</td>
          <td class="mono" style="color:var(--ink-mute)">{{.Prev}}</td>
          <td class="mono">{{.This}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}
    <p class="sub" style="margin-top:12px">No audit records in this run.</p>
    {{end}}
  </section>

  {{if .Errors}}
  <section>
    <h2>Errors <span class="count">{{len .Errors}}</span></h2>
    <p class="sub">Recorded in the order they arrived. Severity drives the left rule color.</p>
    {{range .Errors}}
      <div class="error-card {{.Severity}}">
        <div class="head">
          <span class="sev">{{.Severity}}</span>
          <span class="name">{{.Name}}</span>
          <span class="time">{{.Time}}</span>
        </div>
        <div class="msg">{{.Message}}</div>
        {{if .AttrsRaw}}<div class="attrs">{{.AttrsRaw}}</div>{{end}}
      </div>
    {{end}}
  </section>
  {{end}}

  <section>
    <h2>Event stream <span class="count">filterable, searchable</span></h2>
    <p class="sub">Every record from <code>events.jsonl</code>, embedded directly. Filtering happens locally in your browser.</p>
    <div class="stream" id="stream">
      <div class="controls">
        <input type="search" id="filter" placeholder="Filter by name, attribute, or text…">
        <div class="toggles" id="toggles">
          <span class="toggle active span"      data-cat="span">span</span>
          <span class="toggle active metric"    data-cat="metric">metric</span>
          <span class="toggle active audit"     data-cat="audit">audit</span>
          <span class="toggle active lifecycle" data-cat="lifecycle">lifecycle</span>
          <span class="toggle active error"     data-cat="error">error</span>
        </div>
      </div>
      <div class="results" id="results"></div>
    </div>
  </section>

  <footer class="foot">
    <span>Generated <span class="mono">{{.GeneratedAt}}</span> by loom <span class="mono">v{{.LoomVersion}}</span> · schema <span class="mono">{{.WireSchema}}</span></span>
    <span class="mono">{{.RunID}}</span>
  </footer>

</main>

<script id="loom-events" type="application/json">{{.EventsJSON}}</script>
<script id="loom-audit" type="application/json">{{.AuditJSON}}</script>
<script>
(function(){
  // ── Stream filter ────────────────────────────────────────────────
  var raw = document.getElementById('loom-events').textContent.trim();
  var events = raw ? JSON.parse(raw) : [];
  var input  = document.getElementById('filter');
  var togBox = document.getElementById('toggles');
  var out    = document.getElementById('results');

  var enabled = {span:true, metric:true, audit:true, lifecycle:true, error:true};

  function shortTime(ts) {
    // ISO-8601 -> HH:MM:SS.mmm
    var i = ts.indexOf('T');
    return i === -1 ? ts : ts.slice(i+1, i+13);
  }
  function escapeHTML(s) {
    return s.replace(/[&<>"]/g, function(c){
      return ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'})[c];
    });
  }
  function renderRow(e) {
    var body = e.name;
    if (e.cat === 'span' && e.dur_ns != null) body += ' · ' + (e.dur_ns/1e6).toFixed(2) + 'ms';
    if (e.cat === 'metric' && e.value != null) body += ' = ' + e.value + (e.kind ? ' ('+e.kind+')' : '');
    if (e.cat === 'error' && e.message)        body += ' — ' + e.message;
    if (e.attrs && Object.keys(e.attrs).length) {
      body += '   ' + JSON.stringify(e.attrs);
    }
    return '<div class="row '+e.cat+'">' +
           '<span class="t">'+escapeHTML(shortTime(e.ts))+'</span>' +
           '<span class="c">'+escapeHTML(e.cat)+'</span>' +
           '<span class="b">'+escapeHTML(body)+'</span>' +
           '</div>';
  }
  function render() {
    var q = input.value.trim().toLowerCase();
    var html = '';
    var n = 0;
    for (var i = 0; i < events.length; i++) {
      var e = events[i];
      if (!enabled[e.cat]) continue;
      if (q) {
        var hay = (e.name || '') + ' ' + JSON.stringify(e.attrs || {}) + ' ' + (e.message || '');
        if (hay.toLowerCase().indexOf(q) === -1) continue;
      }
      html += renderRow(e);
      if (++n >= 1000) break;  // safety bound for huge runs
    }
    out.innerHTML = html ||
      '<div class="empty">no events match the current filter</div>';
  }
  input.addEventListener('input', render);
  togBox.addEventListener('click', function(ev){
    var t = ev.target;
    if (!t.classList || !t.classList.contains('toggle')) return;
    var cat = t.getAttribute('data-cat');
    enabled[cat] = !enabled[cat];
    t.classList.toggle('active', enabled[cat]);
    render();
  });
  render();

  // ── Audit chain verifier (SubtleCrypto SHA-256) ──────────────────
  var auditRaw = document.getElementById('loom-audit').textContent;
  var auditLines = auditRaw.split('\n').filter(function(s){return s.trim().length>0;});
  var btn = document.getElementById('verify-btn');
  var stat = document.getElementById('verify-status');
  var headEl = document.getElementById('chain-head');
  var declaredHead = headEl.textContent.trim();

  async function sha256(str) {
    var enc = new TextEncoder().encode(str);
    var hash = await crypto.subtle.digest('SHA-256', enc);
    return Array.from(new Uint8Array(hash))
      .map(function(b){return b.toString(16).padStart(2,'0');})
      .join('');
  }

  async function verifyChain() {
    btn.disabled = true;
    stat.textContent = 'verifying…';
    stat.className = 'verify-status';
    var prev = '0'.repeat(64);
    for (var i = 0; i < auditLines.length; i++) {
      var line = auditLines[i];
      // canonical payload = bytes inside { ... } up to ',"chain":{'
      var open = line.indexOf(',"chain":{');
      if (line[0] !== '{' || open < 0) {
        stat.textContent = '✗ malformed record at index ' + i;
        stat.className = 'verify-status fail';
        btn.disabled = false;
        return;
      }
      var canonical = line.slice(1, open);
      var got = await sha256(prev + canonical);
      var rec = JSON.parse(line);
      if (rec.chain.prev !== prev) {
        stat.textContent = '✗ prev mismatch at seq ' + rec.seq;
        stat.className = 'verify-status fail';
        btn.disabled = false;
        return;
      }
      if (rec.chain.this !== got) {
        stat.textContent = '✗ hash mismatch at seq ' + rec.seq;
        stat.className = 'verify-status fail';
        btn.disabled = false;
        return;
      }
      prev = got;
    }
    var declared = declaredHead;
    if (auditLines.length === 0) declared = '0'.repeat(64);
    var matchesDeclared = (prev === declared) || (auditLines.length === 0 && declared === '0'.repeat(64));
    if (matchesDeclared) {
      stat.textContent = '✓ ' + auditLines.length + ' records · matches declared head';
      stat.className = 'verify-status ok';
    } else {
      // Chain is internally valid but the manifest's recorded head does
      // not match the head computed from the audit records. This is a
      // tampering signal — either the manifest was edited, the chain
      // was truncated, or extra records were appended after finalize.
      // Do NOT report green.
      stat.textContent = '⚠ chain internally valid but manifest head mismatch — '
        + 'computed ' + prev.slice(0,8) + '…' + prev.slice(-4)
        + ', declared ' + declared.slice(0,8) + '…' + declared.slice(-4);
      stat.className = 'verify-status fail';
    }
    btn.disabled = false;
  }
  btn.addEventListener('click', verifyChain);
})();
</script>
</body>
</html>
`
