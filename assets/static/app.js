'use strict';
const $ = (id) => document.getElementById(id);
const state = { maps: [], types: [], indicators: [], visibleTypes: new Set(), showCustom: true, adding: false, pending: null, selected: null, admin: false };

async function api(path, opts) {
  const r = await fetch(path, opts);
  if (!r.ok) {
    let msg = r.statusText;
    try { msg = (await r.json()).error || msg; } catch (e) { /* no body */ }
    throw new Error(msg);
  }
  return r.status === 204 ? null : r.json();
}
const post = (path, body) => api(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
const typeOf = (id) => state.types.find((t) => t.id === id);
const currentMap = () => state.maps.find((m) => m.id === Number($('map-select').value));

async function init() {
  const cfg = await api('/api/config');
  state.admin = cfg.admin;
  $('admin-map-btn').hidden = !state.admin;
  $('admin').hidden = !state.admin;
  if (state.admin) {
    const o = document.createElement('option');
    o.value = 'official'; o.textContent = 'Official indicator (admin)';
    $('f-mode').appendChild(o);
  }
  state.types = await api('/api/types');
  state.types.forEach((t) => state.visibleTypes.add(t.id));
  renderFilters();
  await loadMaps();
}

async function loadMaps(selectId) {
  state.maps = await api('/api/maps');
  const sel = $('map-select');
  sel.replaceChildren();
  let group = null, groupName = null;
  for (const m of state.maps) {
    if (m.game !== groupName) {
      group = document.createElement('optgroup');
      group.label = groupName = m.game;
      sel.appendChild(group);
    }
    const o = document.createElement('option');
    o.value = m.id; o.textContent = m.name;
    group.appendChild(o);
  }
  if (selectId) sel.value = selectId;
  await showMap();
}

async function showMap() {
  const m = currentMap();
  if (!m) return;
  $('map-img').src = m.image;
  state.selected = null;
  renderDetail();
  state.indicators = await api('/api/indicators?map=' + m.id);
  renderMarkers();
  if (state.admin) loadSubmissions();
}

function renderFilters() {
  const box = $('filters');
  box.replaceChildren();
  const fsel = $('f-type');
  fsel.replaceChildren();
  for (const t of state.types) {
    const label = document.createElement('label');
    label.className = 'filter';
    const cb = document.createElement('input');
    cb.type = 'checkbox'; cb.checked = state.visibleTypes.has(t.id);
    cb.onchange = () => { cb.checked ? state.visibleTypes.add(t.id) : state.visibleTypes.delete(t.id); renderMarkers(); };
    const sw = document.createElement('span');
    sw.className = 'swatch'; sw.style.background = t.color;
    label.append(cb, sw, document.createTextNode(t.name));
    box.appendChild(label);
    const o = document.createElement('option');
    o.value = t.id; o.textContent = t.name;
    fsel.appendChild(o);
  }
}

function renderMarkers() {
  const map = $('map');
  map.querySelectorAll('.marker').forEach((e) => e.remove());
  for (const i of state.indicators) {
    if (!state.visibleTypes.has(i.typeId)) continue;
    if (i.source === 'user' && !state.showCustom) continue;
    const t = typeOf(i.typeId) || { color: '#888', icon: '?', name: '' };
    const b = document.createElement('button');
    b.className = 'marker' + (i.source === 'user' ? ' custom' : '') + (state.selected === i.id ? ' selected' : '');
    b.style.left = i.x * 100 + '%';
    b.style.top = i.y * 100 + '%';
    b.style.background = t.color;
    b.textContent = t.icon;
    b.title = i.title || t.name;
    b.onclick = (ev) => { ev.stopPropagation(); state.selected = i.id; renderMarkers(); renderDetail(); };
    map.appendChild(b);
  }
}

function renderDetail() {
  const i = state.indicators.find((x) => x.id === state.selected);
  $('detail').hidden = !i;
  if (!i) return;
  const t = typeOf(i.typeId);
  $('d-title').textContent = i.title || (t && t.name) || 'Indicator';
  $('d-type').textContent = (t ? t.name : '') + (i.source === 'user' ? ' (custom)' : '');
  $('d-details').textContent = i.details;
  $('d-shot').hidden = !i.screenshot;
  if (i.screenshot) $('d-shot').src = i.screenshot;
  $('d-delete').hidden = i.source !== 'user';
}

async function loadSubmissions() {
  const box = $('submissions');
  box.replaceChildren();
  const subs = (await api('/api/admin/submissions')).filter((s) => s.mapId === (currentMap() || {}).id);
  for (const s of subs) {
    const d = document.createElement('div');
    d.className = 'sub';
    const t = typeOf(s.typeId);
    d.append(document.createTextNode(`${t ? t.name : '?'} ${s.title} (${s.x.toFixed(3)}, ${s.y.toFixed(3)}) `));
    if (s.screenshot) { const a = document.createElement('a'); a.href = s.screenshot; a.target = '_blank'; a.textContent = 'screenshot'; d.append(a); }
    if (s.details) { const p = document.createElement('div'); p.className = 'muted'; p.textContent = s.details; d.append(p); }
    const ok = document.createElement('button'); ok.textContent = 'Approve';
    ok.onclick = async () => { await api(`/api/admin/submissions/${s.id}/approve`, { method: 'POST' }); showMap(); };
    const no = document.createElement('button'); no.textContent = 'Reject';
    no.onclick = async () => { await api(`/api/admin/indicators/${s.id}`, { method: 'DELETE' }); showMap(); };
    d.append(document.createElement('br'), ok, no);
    box.appendChild(d);
  }
}

$('map-select').onchange = showMap;
$('show-custom').onchange = (e) => { state.showCustom = e.target.checked; renderMarkers(); };
$('add-btn').onclick = () => {
  state.adding = !state.adding;
  $('add-btn').classList.toggle('active', state.adding);
  $('add-btn').textContent = state.adding ? 'Click the map to place…' : '+ Add indicator';
  $('map').classList.toggle('adding', state.adding);
};
$('map').onclick = (e) => {
  if (!state.adding) return;
  const r = $('map-img').getBoundingClientRect();
  state.pending = { x: Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)), y: Math.min(1, Math.max(0, (e.clientY - r.top) / r.height)) };
  $('indicator-form').reset();
  $('indicator-dlg').showModal();
};
$('indicator-dlg').addEventListener('close', () => {
  state.adding = false;
  $('add-btn').classList.remove('active');
  $('add-btn').textContent = '+ Add indicator';
  $('map').classList.remove('adding');
});
$('indicator-form').onsubmit = async (e) => {
  if (e.submitter && e.submitter.value !== 'ok') return;
  const f = new FormData($('indicator-form'));
  try {
    let screenshot = '';
    const file = f.get('file');
    if (file && file.size) {
      const up = new FormData();
      up.append('file', file);
      screenshot = (await api('/api/screenshots', { method: 'POST', body: up })).url;
    }
    await post('/api/indicators', {
      mapId: currentMap().id, typeId: Number(f.get('typeId')), x: state.pending.x, y: state.pending.y,
      title: f.get('title'), details: f.get('details'), screenshot, mode: f.get('mode'),
    });
    if (f.get('mode') === 'submit') alert('Thanks! Your submission was queued for review.');
    showMap();
  } catch (err) { alert(err.message); }
};
$('d-delete').onclick = async () => {
  await api('/api/indicators/' + state.selected, { method: 'DELETE' });
  showMap();
};
$('admin-map-btn').onclick = () => $('map-dlg').showModal();
$('map-form').onsubmit = async (e) => {
  if (e.submitter && e.submitter.value !== 'ok') return;
  try {
    const m = await api('/api/admin/maps', { method: 'POST', body: new FormData($('map-form')) });
    await loadMaps(m.id);
  } catch (err) { alert(err.message); }
};

init().catch((e) => alert(e.message));
