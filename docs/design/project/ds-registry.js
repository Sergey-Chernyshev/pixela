/* ============================================================
   Pixela Design System — component registry (Storybook stories)
   Each DS[id] = { title, desc, used[], body()->html, init?() }
   ============================================================ */
(function(){
const ic = {
  plus:'<path d="M12 5v14M5 12h14"/>',
  check:'<path d="M20 6 9 17l-5-5"/>',
  x:'<path d="M18 6 6 18M6 6l12 12"/>',
  eye:'<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7z"/><circle cx="12" cy="12" r="3"/>',
  warn:'<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4M12 17h.01"/>',
  spin:'<path d="M21 12a9 9 0 1 1-6.2-8.5"/>',
  search:'<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>',
  chev:'<path d="m6 9 6 6 6-6"/>',
  branch:'<circle cx="6" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M6 9v6"/><path d="M18 6a9 9 0 0 1-9 9"/><circle cx="18" cy="6" r="3"/>',
  grid:'<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
  trash:'<path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>',
};
const svg = (p,sw=2)=>`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="${sw}">${p}</svg>`;
function cell(label, html, sub, stageCls=''){ return `<div class="cell"><div class="stage ${stageCls}">${html}</div><div class="cap">${label}${sub?`<span class="sub">${sub}</span>`:''}</div></div>`; }
function story(title, desc, cells){ return `<div class="story"><h2>${title}</h2>${desc?`<p class="sd">${desc}</p>`:''}<div class="grid">${Array.isArray(cells)?cells.join(''):cells}</div></div>`; }

/* status pill builder */
const statuses = {
  passed:['Пройдено',ic.check], approved:['Одобрено',ic.check], unchanged:['Без изм.',ic.check],
  review:['Нужна проверка',ic.eye], changed:['Изменено',ic.warn], new:['Новый',ic.plus],
  removed:['Удалён','<path d="M5 12h14"/>'], rejected:['Отклонён',ic.x], comparing:['Сравнение',ic.spin],
};
const pill = (k)=>`<span class="status status--${k}">${svg(statuses[k][1],2.2)} ${statuses[k][0]}</span>`;

/* ---------------------------------------------------------- */
const DS = {};

DS.color = { title:'Цвет', desc:'Тёмная палитра с возвышением через светлоту поверхности (не тени). Один приглушённый акцент и desaturated семантика для тёмного фона.',
  body(){
    const sw = (n,v,note)=>`<div class="sw"><div class="chip2" style="background:${v}"></div><div class="meta"><div class="n">${n}</div><div class="v">${note||v}</div></div></div>`;
    return story('Поверхности', 'Каждый уровень — отдельная светлота, граница 1px вместо тени.',
        `<div class="swrow" style="width:100%">
          ${sw('bg','#0E0E10','--bg')}${sw('surface-1','#16161A','--surface-1')}${sw('surface-2','#1E1E24','--surface-2')}${sw('surface-3','#26262E','--surface-3')}${sw('inset','#0B0B0D','--surface-inset')}
        </div>`)
      + story('Текст', '',
        `<div class="swrow" style="width:100%">
          ${sw('text-1','rgba(255,255,255,.92)','92% white')}${sw('text-2','rgba(255,255,255,.60)','60% white')}${sw('text-3','rgba(255,255,255,.40)','40% white')}
        </div>`)
      + story('Акцент', 'Единственный насыщенный цвет в системе.',
        `<div class="swrow" style="width:100%">
          ${sw('accent','#6E8AFA','--accent')}${sw('accent-2','#8AA0FB','--accent-2 (hover)')}${sw('accent-bg','rgba(110,138,250,.14)','--accent-bg')}
        </div>`)
      + story('Семантика', 'Статусы и дифф. Никогда не передаём смысл только цветом — всегда + иконка/текст.',
        `<div class="swrow" style="width:100%">
          ${sw('ok','#4FB58A','passed / approved')}${sw('changed','#E0A53E','changed / review')}${sw('new','#56A8D6','new / info')}${sw('removed','#E06A52','removed / error')}
        </div>`);
  }};

DS.type = { title:'Типографика', desc:'Geist для интерфейса, Geist Mono для кода, хэшей, чисел и табличных значений.',
  body(){
    const row=(lbl,style,txt)=>`<div class="typerow"><span class="lbl">${lbl}</span><span style="${style}">${txt}</span></div>`;
    return story('Шкала (Geist Sans)','',
      `<div style="width:100%">
        ${row('22 / 600','font-size:22px;font-weight:600;letter-spacing:-.02em','Заголовок страницы')}
        ${row('18 / 600','font-size:18px;font-weight:600','Заголовок раздела')}
        ${row('15 / 600','font-size:15px;font-weight:600','Заголовок карточки')}
        ${row('13.5 / 500','font-size:13.5px;font-weight:500','Основной текст интерфейса')}
        ${row('12.5 / 550','font-size:12.5px;font-weight:550;color:var(--text-2)','Подписи и метки')}
        ${row('11 / 600','font-size:11px;font-weight:600;letter-spacing:.06em;text-transform:uppercase;color:var(--text-3)','Надзаголовок (overline)')}
      </div>`)
    + story('Geist Mono','Хэши, размеры, проценты, время — всё с tabular-nums.',
      `<div style="width:100%">
        ${row('mono 13','font-family:var(--mono);font-size:13px','a1b9f3c · 960×880 · 3.47%')}
        ${row('mono 12','font-family:var(--mono);font-size:12px;color:var(--text-2)','pxl_live_••••a39f')}
      </div>`);
  }};

DS.radius = { title:'Радиусы и тени', desc:'Скругления по шкале, возвышение — светлотой и 1px-границей. Тени только для всплывающих слоёв (меню, модалки).',
  body(){
    return story('Радиусы','',
      `<div class="radrow">
        <div class="radbox"><div class="b" style="border-radius:5px"></div><div class="cap">5px<span class="sub">kbd, мелкое</span></div></div>
        <div class="radbox"><div class="b" style="border-radius:6px"></div><div class="cap">6px<span class="sub">--radius-sm</span></div></div>
        <div class="radbox"><div class="b" style="border-radius:8px"></div><div class="cap">8px<span class="sub">--radius, поля</span></div></div>
        <div class="radbox"><div class="b" style="border-radius:12px"></div><div class="cap">12px<span class="sub">--radius-lg, карточки</span></div></div>
        <div class="radbox"><div class="b" style="border-radius:999px"></div><div class="cap">999px<span class="sub">пилюли, свитчи</span></div></div>
      </div>`)
    + story('Возвышение','Всплывающие слои поднимаются мягкой тенью + более светлой границей.',
      `${cell('Карточка','<div class="card-ds" style="width:150px;height:60px"></div>','граница, без тени')}
       ${cell('Меню / модалка','<div style="width:150px;height:60px;border-radius:10px;background:var(--surface-2);border:1px solid var(--border-strong);box-shadow:0 12px 36px rgba(0,0,0,.55)"></div>','тень + светлая граница')}`);
  }};

DS.button = { title:'Кнопки', desc:'Пять вариантов, единая высота 32px. Состояния: hover, pressed, focus, disabled.', used:['везде','actionbar','settings'],
  body(){
    const b=(cls,txt,extra='')=>`<button class="btn ${cls} ${extra}">${txt}</button>`;
    return story('Варианты',null,[
      cell('Вторичная', b('','Сравнить')),
      cell('Основная', b('btn--primary','Принять')),
      cell('Успех', b('btn--ok',`${svg(ic.check,2.4)} Принять`)),
      cell('Опасность', b('btn--danger',`${svg(ic.x,2.2)} Отклонить`)),
      cell('Ghost', b('btn--ghost','Отмена')),
    ])
    + story('Состояния · Основная',null,[
      cell('Default', b('btn--primary','Кнопка')),
      cell('Hover', b('btn--primary','Кнопка','is-hover')),
      cell('Pressed', b('btn--primary','Кнопка','is-active')),
      cell('Focus', b('btn--primary','Кнопка','is-focus')),
      cell('Disabled', b('btn--primary','Кнопка','is-disabled')),
    ])
    + story('Состояния · Вторичная',null,[
      cell('Default', b('','Кнопка')),
      cell('Hover', b('','Кнопка','is-hover')),
      cell('Pressed', b('','Кнопка','is-active')),
      cell('Focus', b('','Кнопка','is-focus')),
      cell('Disabled', b('','Кнопка','is-disabled')),
    ])
    + story('С иконкой, размеры и горячая клавиша',null,[
      cell('Иконка + текст', b('btn--primary',`${svg(ic.plus,2)} Создать`)),
      cell('Маленькая', `<button class="btn" style="height:28px;font-size:12px">Малая</button>`),
      cell('Большая', `<button class="btn btn--ok" style="height:40px;padding:0 22px;font-size:14px">${svg(ic.check,2.4)} Принять <kbd style="background:rgba(0,0,0,.25);color:#06140E">A</kbd></button>`),
    ]);
  }};

DS.iconbtn = { title:'Иконочные кнопки', desc:'Квадратная 30px кнопка для тулбаров. Состояние «нажато» (aria-pressed) — для тумблеров вроде синхролока или режима диффа.', used:['review · тулбар','buildlist'],
  body(){
    const i=(p,extra='')=>`<button class="iconbtn ${extra}">${svg(p)}</button>`;
    return story('Состояния',null,[
      cell('Default', i(ic.plus)),
      cell('Hover', i(ic.plus,'is-hover')),
      cell('Pressed', i(ic.plus,'is-active')),
      cell('Включено', i('<rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>','is-pressed'),'aria-pressed=true'),
    ])
    + story('Группа в тулбаре',null,
      `${cell('Навигация', `<div style="display:flex;gap:4px">${i('<path d="M15 18l-6-6 6-6"/>')}${i('<path d="M9 18l6-6-6-6"/>')}</div>`)}
       ${cell('Зум', `<div style="display:flex;gap:4px;align-items:center">${i('<path d="M5 12h14"/>')}<span class="mono" style="font-size:12px;color:var(--text-2);min-width:40px;text-align:center">Fit</span>${i(ic.plus)}</div>`)}`);
  }};

DS.segmented = { title:'Сегментированный контрол', desc:'Взаимоисключающий выбор 2–3 опций. Активный сегмент подсвечен поверхностью.', used:['review · режимы','projects','members'],
  body(){
    return story('Состояния сегментов',null,[
      cell('2 опции', `<div class="seg"><button class="is-active">Сетка</button><button>Список</button></div>`),
      cell('Hover (неактивный)', `<div class="seg"><button class="is-active">Сетка</button><button class="is-hover">Список</button></div>`),
    ])
    + story('С иконками (режимы сравнения)',null,
      `${cell('3 опции', `<div class="seg">
        <button><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="9" cy="12" r="6"/><circle cx="15" cy="12" r="6"/></svg> Onion</button>
        <button class="is-active"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="4" width="7" height="16" rx="1"/><rect x="14" y="4" width="7" height="16" rx="1"/></svg> Рядом</button>
        <button><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="4" width="18" height="16" rx="1"/><path d="M12 4v16"/></svg> Шторка</button>
      </div>`,'средний активен')}`);
  }};

DS.tabs = { title:'Вкладки', desc:'Горизонтальная навигация с подчёркиванием активной. Использована в деталях сборки.', used:['settings','builddetail'],
  body(){
    return story('Состояния',null,
      `${cell('Набор вкладок', `<div class="tabs"><button class="is-active">Снимки</button><button>Тесты</button><button class="is-hover">История</button><button>Настройки</button></div>`,'1-я активна, 3-я hover',' ')}`);
  }};

DS.navitem = { title:'Навигация (пункт меню)', desc:'Вертикальный пункт бокового меню. Может нести счётчик. Активный — на фоне акцента.', used:['sidebar везде'],
  body(){
    const n=(p,t,extra='',ct='')=>`<div class="navitem ${extra}" style="width:200px">${svg(p)}${t}${ct?`<span class="ct">${ct}</span>`:''}</div>`;
    return story('Состояния',null,[
      cell('Default', n(ic.grid,'Проекты'),'','wide'),
      cell('Hover', n(ic.grid,'Проекты','is-hover'),'','wide'),
      cell('Active', n(ic.grid,'Проекты','is-active'),'','wide'),
      cell('Со счётчиком', n(ic.grid,'Проекты','is-active','6'),'','wide'),
    ]);
  }};

DS.input = { title:'Поле ввода', desc:'Текстовое поле. Состояния: hover, focus, заполнено, ошибка, отключено. Плюс варианты с иконкой и префиксом.', used:['settings','login','поиск'],
  body(){
    const inp=(extra='',val='',ph='you@acme.dev')=>`<input class="input ${extra}" style="width:240px" ${val?`value="${val}"`:`placeholder="${ph}"`}>`;
    return story('Состояния',null,[
      cell('Default', inp(),'','wide'),
      cell('Hover', inp('is-hover'),'','wide'),
      cell('Focus', inp('is-focus'),'','wide'),
      cell('Заполнено', inp('','jordan@acme.dev'),'','wide'),
      cell('Ошибка', `<div style="width:240px">${inp('is-error','неверный e-mail')}<div class="input-help err">Введите рабочую почту</div></div>`,'','wide tall'),
      cell('Disabled', inp('is-disabled'),'','wide'),
    ])
    + story('Варианты',null,[
      cell('С иконкой', `<div class="input-wrap" style="width:240px"><span class="lead">${svg(ic.search,2)}</span><input class="input" placeholder="Поиск по коммиту…"></div>`,'','wide'),
      cell('С префиксом', `<div class="input-group" style="width:240px"><span class="pre">pixela.dev/</span><input class="input mono" value="acme"></div>`,'','wide'),
      cell('Textarea', `<textarea class="input" style="width:240px" rows="3">Снимок промокода после применения SAVE10.</textarea>`,'','wide tall'),
    ]);
  }};

DS.select = { title:'Селект / Дропдаун', desc:'Кнопка-триггер + всплывающее меню. Показаны все состояния триггера и опций. Снизу — живой пример.', used:['filters','project switcher','settings'],
  body(){
    const sel=(extra='',val='Chromium · Desktop',ph=false)=>`<div class="select ${extra} ${ph?'is-placeholder':''}" style="min-width:200px"><span class="val">${val}</span><span class="caret">${svg(ic.chev,2)}</span></div>`;
    return story('Состояния триггера',null,[
      cell('Default', sel(),'','wide'),
      cell('Hover', sel('is-hover'),'','wide'),
      cell('Focus', sel('is-focus'),'','wide'),
      cell('Open', sel('is-open'),'каретка ↑','wide'),
      cell('Placeholder', sel('','Выберите браузер…',true),'','wide'),
      cell('Disabled', sel('is-disabled'),'','wide'),
    ])
    + story('Меню — состояния опций',null,
      `${cell('Открытое меню', `<div class="menu" style="width:220px">
        <div class="grouplbl">Браузер</div>
        <div class="opt is-selected">${svg('<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="3"/>')} Chromium <span class="tick">${svg(ic.check,2.4)}</span></div>
        <div class="opt is-hover">${svg('<circle cx="12" cy="12" r="9"/>')} Firefox</div>
        <div class="opt">${svg('<circle cx="12" cy="12" r="9"/>')} WebKit</div>
        <div class="sep"></div>
        <div class="opt is-disabled">${svg('<rect x="7" y="2" width="10" height="20" rx="2"/>')} Mobile Safari <span style="margin-left:auto;font-size:10px">скоро</span></div>
      </div>`,'selected · hover · disabled','tall')}`)
    + story('Живой пример','Клик открывает и закрывает меню.',
      `${cell('Интерактивный', `<div id="liveselectwrap" style="position:relative;width:200px">
        <div class="select" id="liveselect" style="min-width:200px"><span class="val" id="liveval">Chromium</span><span class="caret">${svg(ic.chev,2)}</span></div>
        <div class="menu" id="livemenu" style="position:absolute;top:44px;left:0;right:0;display:none">
          <div class="opt" data-v="Chromium">${svg('<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="3"/>')} Chromium <span class="tick">${svg(ic.check,2.4)}</span></div>
          <div class="opt" data-v="Firefox">${svg('<circle cx="12" cy="12" r="9"/>')} Firefox <span class="tick">${svg(ic.check,2.4)}</span></div>
          <div class="opt" data-v="WebKit">${svg('<circle cx="12" cy="12" r="9"/>')} WebKit <span class="tick">${svg(ic.check,2.4)}</span></div>
        </div>
      </div>`,'кликни','tall wide')}`);
  },
  init(){
    const sel=document.getElementById('liveselect'), menu=document.getElementById('livemenu'), val=document.getElementById('liveval');
    if(!sel) return;
    let open=false; const setOpen=(o)=>{ open=o; menu.style.display=o?'block':'none'; sel.classList.toggle('is-open',o); };
    const mark=()=>menu.querySelectorAll('.opt').forEach(o=>o.classList.toggle('is-selected', o.dataset.v===val.textContent));
    mark();
    sel.addEventListener('click',(e)=>{ e.stopPropagation(); setOpen(!open); });
    menu.querySelectorAll('.opt').forEach(o=> o.addEventListener('click',()=>{ val.textContent=o.dataset.v; mark(); setOpen(false); }));
    document.addEventListener('click',()=>setOpen(false));
  }};

DS.switch = { title:'Переключатель', desc:'Бинарный тумблер. Состояния вкл/выкл с hover, focus и disabled. Снизу — живой.', used:['settings','builddetail'],
  body(){
    const s=(extra='')=>`<div class="switch ${extra}"></div>`;
    return story('Состояния',null,[
      cell('Выкл', s()),
      cell('Выкл · hover', s('is-hover')),
      cell('Вкл', s('on')),
      cell('Вкл · hover', s('on is-hover')),
      cell('Focus', s('on is-focus')),
      cell('Disabled', s('on is-disabled')),
    ])
    + story('Живой пример',null,
      `${cell('Кликни', `<div style="display:flex;align-items:center;gap:12px"><div class="switch on" id="liveswitch"></div><span id="liveswitchlbl" style="font-size:13px;color:var(--text-2)">Вкл</span></div>`)}`);
  },
  init(){ const sw=document.getElementById('liveswitch'); if(!sw) return; const lbl=document.getElementById('liveswitchlbl'); sw.addEventListener('click',()=>{ sw.classList.toggle('on'); lbl.textContent=sw.classList.contains('on')?'Вкл':'Выкл'; }); }};

DS.check = { title:'Чекбокс', desc:'Множественный выбор. Для выделения снимков в сетке и фильтров.', used:['builddetail (выбор)'],
  body(){
    const c=(extra='')=>`<div class="check ${extra}">${svg(ic.check,3)}</div>`;
    return story('Состояния',null,[
      cell('Выкл', c()),
      cell('Hover', c('is-hover')),
      cell('Вкл', c('on')),
      cell('Focus', c('on is-focus')),
    ]);
  }};

DS.slider = { title:'Слайдер', desc:'Непрерывное значение: порог диффа, прозрачность onion-skin, допуск анти-алиасинга.', used:['settings · пороги','review · onion'],
  body(){
    const r=(fill,extra='')=>`<div style="width:200px"><input type="range" class="range ${extra}" value="${fill}" style="--fill:${fill}%"></div>`;
    return story('Позиции и фокус',null,[
      cell('10%', r(10),'строгий порог','wide'),
      cell('40%', r(40),'','wide'),
      cell('80%', r(80),'','wide'),
      cell('Focus', r(40,'is-focus'),'кольцо на ручке','wide'),
    ])
    + story('С подписью значения',null,
      `${cell('Порог различий', `<div style="display:flex;align-items:center;gap:14px;width:240px"><input type="range" class="range" value="20" style="--fill:20%;flex:1"><span class="mono" style="font-size:14px;font-weight:600;color:var(--accent-2);min-width:46px;text-align:right">0.20</span></div>`,'','wide')}`);
  }};

DS.status = { title:'Статусы', desc:'Пилюля статуса = иконка + текст + цвет (никогда только цвет). Покрывает прогон CI и состояние снимка.', used:['buildlist','builddetail','review'],
  body(){
    return story('Прогон сборки',null,['review','passed','rejected','comparing','error'].map(k=> cell(statuses[k]?statuses[k][0]:'Ошибка', k==='error'?`<span class="status status--removed">${svg(ic.warn,2.2)} Ошибка</span>`:pill(k))))
    + story('Состояние снимка',null,['unchanged','changed','new','removed','approved'].map(k=> cell(statuses[k][0], pill(k))))
    + story('Доп. маркеры',null,[
      cell('Нестабильный', `<span class="status status--changed"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/></svg> Нестабильный</span>`),
    ]);
  }};

DS.chip = { title:'Чипы-счётчики', desc:'Компактный счётчик с цветной точкой. Сводка по снимкам в строке сборки.', used:['buildlist','builddetail'],
  body(){
    const c=(cls,n,t)=>`<span class="chip chip--${cls}"><span class="dot"></span><b>${n}</b>${t?' '+t:''}</span>`;
    return story('Варианты',null,[
      cell('Без изменений', c('ok','480','без изм.')),
      cell('Изменено', c('changed','12','изм.')),
      cell('Новые', c('new','3','нов.')),
      cell('Удалённые', c('removed','1','уд.')),
    ])
    + story('В строке',null,`${cell('Сводка сборки', `<div style="display:flex;gap:6px">${c('ok','480')}${c('changed','12')}${c('new','3')}${c('removed','1')}</div>`,'','wide')}`);
  }};

DS.role = { title:'Роли', desc:'Бейдж роли участника команды. Цвет кодирует уровень доступа, иконка дублирует смысл.', used:['members'],
  body(){
    const r=(cls,p,t)=>`<span class="role role--${cls}">${svg(p)}${t}</span>`;
    return story('Роли доступа',null,[
      cell('Админ', r('admin','<path d="M12 2 4 6v6c0 5 3.4 8.5 8 10 4.6-1.5 8-5 8-10V6z"/>','Админ')),
      cell('Ревьюер', r('reviewer',ic.eye,'Ревьюер')),
      cell('Разработчик', r('dev','<path d="m8 9-4 3 4 3M16 9l4 3-4 3M13 6l-2 12"/>','Разработчик')),
      cell('Наблюдатель', r('viewer',ic.eye,'Наблюдатель')),
    ]);
  }};

DS.avatar = { title:'Аватары', desc:'Инициалы на цветном фоне. Размеры xs–lg и стопка для команд проекта.', used:['members','projects','review · история'],
  body(){
    const a=(sz,i,c)=>`<span class="avatar avatar--${sz}" style="background:${c}">${i}</span>`;
    return story('Размеры',null,[
      cell('xs · 20', a('xs','МК','#5b6ee0')),
      cell('sm · 24', a('sm','ТР','#c2683a')),
      cell('md · 32', a('md','АЛ','#3a8f6b')),
      cell('lg · 40', a('lg','JA','#7a5bd0')),
    ])
    + story('Стопка команды',null,[
      cell('4 участника', `<div class="avatar-stack">${a('sm','JA','#5b6ee0')}${a('sm','МК','#5b6ee0')}${a('sm','ТР','#c2683a')}${a('sm','АЛ','#3a8f6b')}</div>`),
      cell('С переполнением', `<div class="avatar-stack">${a('sm','МК','#5b6ee0')}${a('sm','ТР','#c2683a')}${a('sm','АЛ','#3a8f6b')}${a('sm','+3','#3a3a44')}</div>`),
    ]);
  }};

DS.kbd = { title:'Горячие клавиши', desc:'Клавиша интерфейса. Разбор снимков полностью клавиатурный: A/R, стрелки, 1/2/3.', used:['review','actionbar'],
  body(){
    return story('Клавиши',null,[
      cell('Принять', '<kbd>A</kbd>'),
      cell('Отклонить', '<kbd>R</kbd>'),
      cell('Навигация', '<div style="display:flex;gap:4px"><kbd>←</kbd><kbd>→</kbd></div>'),
      cell('Режимы', '<div style="display:flex;gap:4px"><kbd>1</kbd><kbd>2</kbd><kbd>3</kbd></div>'),
      cell('Сочетание', '<div style="display:flex;gap:5px;align-items:center"><kbd>⌘</kbd><span style="color:var(--text-3)">+</span><kbd>K</kbd></div>'),
    ]);
  }};

DS.sparkline = { title:'Спарклайны и полосы', desc:'Микрографики тренда: статус прогонов во времени и здоровье проекта (stacked bar).', used:['projects','members','testhistory'],
  body(){
    const bars=(arr)=>`<div style="display:flex;align-items:flex-end;gap:2px;height:32px">${arr.map(([t,h])=>`<i style="width:5px;border-radius:1.5px 1.5px 0 0;height:${h}px;background:var(--${t==='c'?'changed':t==='r'?'removed':'ok'})"></i>`).join('')}</div>`;
    const d=[['p',9],['p',7],['p',8],['c',24],['p',9],['p',7],['r',30],['p',8],['p',9],['p',7],['p',8],['c',22],['p',9],['p',8]];
    return story('Тренд прогонов',null,[
      cell('Здоровый', bars([['p',9],['p',7],['p',8],['p',9],['p',7],['p',8],['p',9],['p',8],['p',7],['p',9]]),'все зелёные','wide'),
      cell('С изменениями', bars(d),'жёлтые/красные пики','wide'),
    ])
    + story('Полоса здоровья проекта',null,
      `${cell('472 / 496 · 95.2%', `<div style="width:240px"><div style="height:6px;border-radius:3px;background:var(--surface-3);overflow:hidden;display:flex"><i style="background:var(--ok);width:95%"></i><i style="background:var(--changed);width:4%"></i><i style="background:var(--removed);width:1%"></i></div></div>`,'ok / changed / removed','wide')}`);
  }};

DS.tooltip = { title:'Тултип', desc:'Подсказка при наведении на иконку или сокращение.',
  body(){ return story('Тултип',null,[ cell('Над элементом', `<div class="tip">Скопировать хэш</div>`,'',' ') ]); }};

DS.card = { title:'Карточка', desc:'Базовый контейнер. При наведении — более светлая граница и лёгкий подъём.', used:['projects','builddetail','settings'],
  body(){
    return story('Состояния',null,[
      cell('Default', `<div class="card-ds" style="width:180px;height:80px"></div>`,'','tall'),
      cell('Hover', `<div class="card-ds is-hover" style="width:180px;height:80px"></div>`,'подъём + граница','tall'),
    ]);
  }};

DS.modal = { title:'Модальное окно', desc:'Диалог поверх затемнённого оверлея. Подтверждения и опасные действия. Снизу — живой пример.', used:['settings · опасная зона','пакетные действия'],
  body(){
    const dlg=(di,icn,h,p,btns)=>`<div class="dialog" style="width:360px"><div class="dh"><span class="di di--${di}">${svg(icn,2.2)}</span><div><h3>${h}</h3><p>${p}</p></div></div><div class="df">${btns}</div></div>`;
    return story('Типы',null,[
      cell('Подтверждение', dlg('accent',ic.check,'Принять 12 изменений?','Снимки станут новой базовой линией для ветки.',`<button class="btn btn--ghost">Отмена</button><button class="btn btn--primary">Принять все</button>`),'','tall wide'),
      cell('Опасное действие', dlg('danger',ic.trash,'Удалить организацию?','Все проекты, снимки и история Acme Inc будут удалены безвозвратно.',`<button class="btn btn--ghost">Отмена</button><button class="btn btn--danger">Удалить</button>`),'','tall wide'),
    ])
    + story('Живой пример','Кнопка открывает оверлей поверх этой области.',
      `${cell('Открыть', `<button class="btn btn--danger" id="openmodal">${svg(ic.trash,2.2)} Удалить проект</button>`)}`);
  },
  init(){
    const btn=document.getElementById('openmodal'); if(!btn) return;
    const body=document.getElementById('sbbody');
    btn.addEventListener('click',()=>{
      const ov=document.createElement('div'); ov.className='overlay';
      ov.innerHTML=`<div class="dialog"><div class="dh"><span class="di di--danger">${svg(ic.trash,2.2)}</span><div><h3>Удалить acme/storefront?</h3><p>Проект, все его снимки и базовые линии будут удалены. Это действие необратимо.</p></div></div><div class="db"><input class="input" placeholder="Введите storefront для подтверждения"></div><div class="df"><button class="btn btn--ghost" data-close>Отмена</button><button class="btn btn--danger">Удалить навсегда</button></div></div>`;
      body.appendChild(ov);
      const close=()=>ov.remove();
      ov.addEventListener('click',(e)=>{ if(e.target===ov||e.target.hasAttribute('data-close')) close(); });
    });
  }};

DS.radio = { title:'Radio', desc:'Взаимоисключающий выбор из группы. Для режимов сравнения, выбора ветки в формах.', used:['settings','фильтры'],
  body(){
    const r=(extra,label)=>`<label class="radio ${extra}"><span class="dot"></span>${label}</label>`;
    return story('Состояния',null,[
      cell('Выкл', r('','Side-by-side')),
      cell('Hover', r('is-hover','Side-by-side')),
      cell('Вкл', r('on','Side-by-side')),
      cell('Focus', r('on is-focus','Side-by-side')),
      cell('Disabled', r('is-disabled','Недоступно')),
    ])
    + story('Группа',null,`${cell('Режим сравнения', `<div class="radio-group">${r('on','Рядом (2-up)')}${r('','Onion-skin')}${r('','Шторка')}</div>`,'','tall wide')}`);
  },
  code:`<label class="radio on">\n  <span class="dot"></span>\n  Рядом (2-up)\n</label>` };

DS.breadcrumb = { title:'Хлебные крошки', desc:'Путь до текущего экрана. В Pixela дублирует иерархию Playwright: spec › describe › test.', used:['testhistory','builddetail'],
  body(){
    return story('Варианты',null,[
      cell('Простые', `<nav class="breadcrumb"><a href="#">Сборки</a><span class="sep">/</span><span class="here">feat/checkout</span></nav>`,'','wide'),
      cell('Иерархия теста', `<nav class="breadcrumb mono"><a href="#">checkout-mobile.spec.ts</a><span class="sep">›</span><a href="#">Checkout flow</a><span class="sep">›</span><span class="here">Payment</span></nav>`,'','wide'),
    ]);
  },
  code:`<nav class="breadcrumb">\n  <a href="#">Сборки</a>\n  <span class="sep">/</span>\n  <span class="here">feat/checkout</span>\n</nav>` };

DS.pager = { title:'Пагинация', desc:'Постраничная навигация для длинных списков снимков и сборок.', used:['snapshots','buildlist'],
  body(){
    const b=(t,extra='')=>`<button class="${extra}">${t}</button>`;
    return story('Состояния кнопок',null,[
      cell('Default', b('2')),
      cell('Hover', b('2','is-hover')),
      cell('Активная', b('2','is-active')),
      cell('Disabled', b('‹','is-disabled')),
    ])
    + story('Полный набор',null,
      `${cell('Пагинатор', `<div class="pager">
        <button class="is-disabled"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 18l-6-6 6-6"/></svg></button>
        <button class="is-active">1</button><button>2</button><button>3</button><span class="gap">…</span><button>17</button>
        <button><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg></button>
      </div>`,'','wide')}`);
  },
  code:`<div class="pager">\n  <button class="is-disabled">‹</button>\n  <button class="is-active">1</button>\n  <button>2</button>\n  <button>3</button>\n  <span class="gap">…</span>\n  <button>›</button>\n</div>` };

DS.toast = { title:'Тосты', desc:'Временное уведомление о результате действия: принятие, ошибка CI, инфо. Снизу — живой пример.', used:['действия принять/отклонить'],
  body(){
    const t=(cls,icn,tt,td,actions='')=>`<div class="toast toast--${cls}"><span class="ti">${svg(icn,2.2)}</span><div class="tc"><div class="tt">${tt}</div><div class="td">${td}</div>${actions}</div><span class="tx">${svg(ic.x,2)}</span></div>`;
    return story('Типы',null,[
      cell('Успех', t('ok',ic.check,'12 снимков принято','Базовая линия обновлена для feat/checkout.'),'','tall wide'),
      cell('Ошибка', t('err',ic.warn,'Сборка упала','Таймаут рендера в WebKit на 3 снимках.'),'','tall wide'),
      cell('С действием', t('info',ic.eye,'Изменение отклонено','search-overlay помечен как регрессия.',`<div class="ta"><a>Отменить</a></div>`),'','tall wide'),
    ])
    + story('Живой пример',null,`${cell('Показать тост', `<button class="btn btn--primary" id="showtoast">${svg(ic.check,2.4)} Принять снимок</button>`)}`);
  },
  init(){
    const btn=document.getElementById('showtoast'); if(!btn) return;
    const body=document.getElementById('sbbody');
    let host=document.getElementById('toasthost');
    if(!host){ host=document.createElement('div'); host.id='toasthost'; host.style.cssText='position:absolute;right:24px;bottom:24px;display:flex;flex-direction:column;gap:10px;z-index:60'; body.appendChild(host); }
    btn.addEventListener('click',()=>{
      const el=document.createElement('div'); el.className='toast toast--ok';
      el.style.cssText='animation:tin .25s ease';
      el.innerHTML=`<span class="ti">${svg(ic.check,2.2)}</span><div class="tc"><div class="tt">Снимок принят</div><div class="td">promo-applied.png стал новой базой.</div></div><span class="tx">${svg(ic.x,2)}</span>`;
      host.appendChild(el);
      el.querySelector('.tx').addEventListener('click',()=>el.remove());
      setTimeout(()=>el.remove(), 3200);
    });
  },
  code:`<div class="toast toast--ok">\n  <span class="ti"><!-- icon --></span>\n  <div class="tc">\n    <div class="tt">12 снимков принято</div>\n    <div class="td">Базовая линия обновлена.</div>\n  </div>\n  <span class="tx"><!-- close --></span>\n</div>` };

DS.empty = { title:'Пустые состояния', desc:'Экран без данных: первый запуск, пустой фильтр, подключение CI. Всегда даёт следующий шаг.', used:['buildlist','snapshots'],
  body(){
    return story('Варианты',null,[
      cell('Нет сборок', `<div class="empty-state"><div class="ei">${svg('<path d="M3 12h4l3 8 4-16 3 8h4"/>',1.7)}</div><h3>Пока нет ни одной сборки</h3><p>Подключите CI, чтобы Pixela получал снимки после каждого прогона.</p><div class="cmd"><span class="pr">$ </span>npx pixela upload ./test-results</div><button class="btn btn--primary">Подключить CI</button></div>`,'','tall wide'),
      cell('Ничего не найдено', `<div class="empty-state"><div class="ei">${svg(ic.search,1.7)}</div><h3>Ничего не найдено</h3><p>По запросу нет снимков. Попробуйте изменить фильтры или сбросить поиск.</p><button class="btn">Сбросить фильтры</button></div>`,'','tall wide'),
    ]);
  },
  code:`<div class="empty-state">\n  <div class="ei"><!-- icon --></div>\n  <h3>Пока нет ни одной сборки</h3>\n  <p>Подключите CI, чтобы получать снимки.</p>\n  <button class="btn btn--primary">Подключить CI</button>\n</div>` };

/* ---- code snippets for existing components ---- */
const CODES = {
  button:`<button class="btn btn--primary">Принять</button>\n<button class="btn btn--danger">Отклонить</button>\n<button class="btn">Сравнить</button>`,
  iconbtn:`<button class="iconbtn" aria-pressed="true">\n  <svg>…</svg>\n</button>`,
  segmented:`<div class="seg">\n  <button aria-pressed="true">Рядом</button>\n  <button>Onion</button>\n</div>`,
  tabs:`<div class="tabs">\n  <button aria-selected="true">Снимки</button>\n  <button>Тесты</button>\n</div>`,
  navitem:`<a class="navitem active">\n  <svg>…</svg> Проекты <span class="ct">6</span>\n</a>`,
  input:`<input class="input" placeholder="you@acme.dev">\n<div class="input-wrap">\n  <span class="lead"><svg>…</svg></span>\n  <input class="input" placeholder="Поиск…">\n</div>`,
  select:`<div class="select">\n  <span class="val">Chromium</span>\n  <span class="caret"><svg>…</svg></span>\n</div>\n<div class="menu">\n  <div class="opt is-selected">Chromium <span class="tick">✓</span></div>\n  <div class="opt">Firefox</div>\n</div>`,
  switch:`<div class="switch on"></div>`,
  check:`<div class="check on"><svg>✓</svg></div>`,
  slider:`<input type="range" class="range" value="20" style="--fill:20%">`,
  status:`<span class="status status--changed">\n  <svg>…</svg> Изменено\n</span>`,
  chip:`<span class="chip chip--changed">\n  <span class="dot"></span><b>12</b> изм.\n</span>`,
  role:`<span class="role role--admin"><svg>…</svg> Админ</span>`,
  avatar:`<span class="avatar avatar--md" style="background:#5b6ee0">МК</span>\n<div class="avatar-stack">…</div>`,
  kbd:`<kbd>A</kbd> <kbd>R</kbd>`,
  card:`<div class="card-ds">…</div>`,
  modal:`<div class="overlay">\n  <div class="dialog">\n    <div class="dh">…</div>\n    <div class="df">\n      <button class="btn btn--ghost">Отмена</button>\n      <button class="btn btn--danger">Удалить</button>\n    </div>\n  </div>\n</div>`,
  tooltip:`<div class="tip">Скопировать хэш</div>`,
};
Object.keys(CODES).forEach(k=>{ if(DS[k]) DS[k].code = CODES[k]; });

window.DS = DS;
})();
