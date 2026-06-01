// Shared project-level sidebar for Pixela (a project = a repository).
// Usage: <aside class="side" id="projside"></aside> then renderProjSide('builds'|'queue'|'tests'|'snapshots'|'baselines'|'settings').
function renderProjSide(active){
  const ic = {
    builds:'<path d="M3 12h4l3 8 4-16 3 8h4"/>',
    queue:'<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7z"/><circle cx="12" cy="12" r="3"/>',
    tests:'<path d="M9 3v6l-5 9a2 2 0 0 0 2 3h12a2 2 0 0 0 2-3l-5-9V3"/><path d="M8 3h8M9.5 13h5"/>',
    snapshots:'<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-5-5L5 21"/>',
    baselines:'<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>',
    settings:'<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>',
  };
  const links = [
    ['builds','Сборки','buildlist.html',''],
    ['queue','Очередь проверок','reviewqueue.html','12'],
    ['tests','Тесты','testtree.html',''],
    ['snapshots','Снимки','snapshots.html',''],
    ['baselines','Базовые линии','baselines.html',''],
    ['settings','Настройки','settings.html',''],
  ];
  const navHTML = links.map(([k,label,href,ct])=>
    `<a href="${href}" class="${k===active?'active':''}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">${ic[k]}</svg>${label}${ct?`<span class="ct ${k==='queue'?'amber':''}">${ct}</span>`:''}</a>`
  ).join('');
  document.getElementById('projside').innerHTML = `
    <div class="brand"><span class="logo"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><rect x="3" y="3" width="8" height="8" rx="1.5"/><rect x="13" y="3" width="8" height="8" rx="1.5"/><rect x="3" y="13" width="8" height="8" rx="1.5"/><rect x="13" y="13" width="8" height="8" rx="1.5" fill="currentColor"/></svg></span><span class="name">Pixela</span></div>
    <div class="backlink" onclick="location.href='projects.html'"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 18l-6-6 6-6"/></svg>Все проекты</div>
    <div class="proj">
      <span class="sq" style="background:linear-gradient(135deg,#6E8AFA,#9d7bf0)"></span>
      <span class="t"><div class="o">Storefront</div><div class="p">acme/storefront</div></span>
      <span class="ch"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m8 9 4-4 4 4M8 15l4 4 4-4"/></svg></span>
    </div>
    <nav class="nav">
      <div class="lbl">Проект</div>
      ${navHTML}
    </nav>
    <div class="foot">
      <span class="ava">МК</span>
      <div><div class="un">Мира Кан</div><div class="ue">mira@acme.dev</div></div>
    </div>`;
}
