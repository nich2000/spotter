(() => {
  const $ = id => document.getElementById(id);
  const esc = escapeHTML;
  const names = {inbox:'Inbox',backlog:'Отложено',ready:'Подготовлено',doing:'В работе',waiting:'Ожидание',done:'Готово'};
  const sources = {manual:'Вручную',calendar:'Календарь',reminders:'Напоминания',mail:'Почта',notes:'Заметки'};
  let snapshot, editing, editingVersion, busy = false;
  const localDate = () => new Intl.DateTimeFormat('sv-SE', {timeZone:snapshot?.data.settings.zone || 'Europe/Moscow'}).format(new Date());
  $('workDate').value = localDate();
  function feedback(text) { $('workFeedback').textContent=text; $('workFeedback').hidden=!text; }
  function setView(view) {
    document.querySelectorAll('[data-work-view]').forEach(n=>n.hidden=n.dataset.workView!==view);
    document.querySelectorAll('.work-tabs [data-view]').forEach(n=>n.setAttribute('aria-pressed',String(n.dataset.view===view)));
  }
  document.querySelectorAll('[data-view]').forEach(n=>n.addEventListener('click',()=>setView(n.dataset.view)));
  async function request(command) {
    const response=await fetch(`/api/workspace?date=${encodeURIComponent($('workDate').value)}`,command ? {method:'POST',headers:{'Content-Type':'application/json','X-Spotter-Request':'workspace'},body:JSON.stringify({...command,version:command.version ?? snapshot.data.version})}:{});
    if(!response.ok) { const error=new Error(await response.text()); error.status=response.status; throw error; }
    return response.json();
  }
  async function load() {if(busy || $('taskDialog').open || $('workspace').contains(document.activeElement) && /INPUT|TEXTAREA|SELECT/.test(document.activeElement.tagName))return;try {snapshot=await request();render();} catch(e){feedback(`Рабочая панель недоступна: ${e.message}`);}}
  async function mutate(command, version) {
    if(busy) return false;
    if(!snapshot){feedback('Сначала загрузите рабочую панель.');return false;}
    busy=true;
    try {snapshot=await request({...command, version});render();feedback('');return true;} catch(e){ if(e.status===409){ try{snapshot=await request();render()}catch{} } const message=e.status===409?'Данные изменились. Закройте и заново откройте карточку перед сохранением. Ваш текст пока остаётся в форме.':e.message;feedback(message);$('taskError').textContent=message;return false;} finally{busy=false;}
  }
  function taskCard(t,actions=true) {
    return `<article class="task-card"><div class="task-tags"><span>${esc(t.project || 'Разовое дело')}</span>${t.mitDate ? `<span class="badge">${t.mainDate===t.mitDate ? 'Главный' : 'В плане'} ${esc(t.mitDate)}</span>`:''}${t.weekly ? '<span class="badge">Неделя</span>':''}${t.status==='done'?'<span class="badge">Готово</span>':''}</div><strong>${esc(t.title)}</strong><p class="helper">${t.minutes} мин · ${esc(sources[t.source]||t.source)}</p>${t.resume?`<p class="resume-note">${esc(t.resume)}</p>`:''}${t.status==='waiting'?`<p class="helper">${esc(t.waitingOn||'Причина не указана')} · Проверить: ${esc(t.checkDate||'дата не указана')}</p>`:''}${actions?`<div class="task-actions"><button type="button" data-edit="${esc(t.id)}">Изменить</button>${t.status!=='done'?`<button type="button" data-done="${esc(t.id)}">Готово</button>`:''}${t.mitDate=== $('workDate').value && ['ready','doing'].includes(t.status) ? `<button type="button" class="primary" data-focus="${esc(t.id)}">Начать блок</button>`:''}</div>`:''}</article>`;
  }
  function renderCandidates() {
    if(!snapshot)return;
    const filter=$('candidateSource').value;
    const items=snapshot.candidates.filter(c=>!filter||filter===c.source);
    $('candidateCount').textContent=`· ${snapshot.candidates.length}`;
    $('restoreCandidates').hidden=!(snapshot.data.dismissed||[]).length;
    $('candidates').innerHTML=items.length?items.map(c=>`<article class="candidate"><div><strong>${esc(c.title)}</strong><p class="helper">${esc(sources[c.source])}${c.project?` · ${esc(c.project)}`:''}${c.count>1?` · ${c.count} уведомления`:''}</p>${c.details?`<details><summary>Исходные сообщения</summary><p class="resume-note">${esc(c.details)}</p></details>`:''}</div><div class="task-actions"><button type="button" data-import="${esc(c.key)}">В Inbox</button><button type="button" data-dismiss="${esc(c.key)}">Пропустить</button></div></article>`).join(''):empty('Новых предложений в этом источнике нет.');
  }
  function renderBoard() {
    if(!snapshot)return;const project=$('projectFilter').value;
    $('kanban').innerHTML=Object.entries(names).filter(([key])=>key!=='inbox').map(([key,name])=>{const all=snapshot.data.tasks.filter(t=>t.status===key),tasks=all.filter(t=>!project||t.project===project);return `<section class="kanban-column"><h3>${name} <span>${all.length}${key==='ready'?' / 15':''}</span></h3>${tasks.map(t=>taskCard(t)).join('')||'<p class="helper">Пока пусто</p>'}</section>`}).join('');
  }
  function render() {
    const {data:d,schedule:p}=snapshot,tasks=d.tasks||[];
    const clock=new Intl.DateTimeFormat('ru-RU',{hour:'2-digit',minute:'2-digit',timeZone:d.settings.zone});
    $('inboxTotal').textContent=tasks.filter(t=>t.status==='inbox').length;
    const daily=tasks.filter(t=>t.mitDate===p.date && ['ready','doing','done'].includes(t.status));
    const main=daily.find(t=>t.mainDate===p.date);
    const wip=tasks.filter(t=>['doing','waiting'].includes(t.status));
    const committed=tasks.filter(t=>t.mitDate===p.date&&['ready','doing'].includes(t.status)).reduce((n,t)=>n+t.minutes,0);
    $('workMetrics').innerHTML=[['Свободно по календарю',p.available?`${p.free} мин`:'Нет данных'],['Бюджет с резервом',p.available?`${p.budget} мин`:'Не рассчитан'],['Выбрано работы',`${committed} мин`],['Начато, включая ожидание',`${wip.length}`]].map(([title,value])=>`<div><span>${title}</span><strong>${value}</strong></div>`).join('');
    $('mitTasks').innerHTML=main?taskCard(main):empty('Выберите один конкретный результат. Не нужно заполнять весь день.');
    $('extraTasks').innerHTML=daily.filter(t=>t!==main).map(t=>taskCard(t)+(!main&&t.status!=='done'?`<button type="button" data-main="${esc(t.id)}">Сделать главным</button>`:'')).join('')||empty('Дополнительных задач нет — это нормально.');
    $('dailyChoices').innerHTML=tasks.filter(t=>['ready','doing'].includes(t.status)).map(t=>`<article class="candidate"><div><strong>${esc(t.title)}</strong><p class="helper">${esc(t.project||'Разовое дело')} · ${t.minutes} мин${t.mitDate&&t.mitDate!==p.date?` · перенос с ${esc(t.mitDate)}`:''}</p></div><div class="task-actions"><button type="button" data-main="${esc(t.id)}">Главный результат</button>${t.mitDate!==p.date?`<button type="button" data-daily="${esc(t.id)}">Дополнительно</button>`:''}</div></article>`).join('')||empty('Пока нет подготовленных задач. Запишите действие в строке выше и разберите его во входящих.');
    $('waitingTasks').innerHTML=wip.filter(t=>t.status==='waiting').map(t=>taskCard(t)).join('')||empty('Сейчас нет задач в ожидании.');
    $('inboxTasks').innerHTML=tasks.filter(t=>t.status==='inbox').map(t=>taskCard(t)).join('')||empty('Принятых входящих нет. Предложения из источников ниже разбираются отдельно.');
    const projects=[...new Set(tasks.map(t=>t.project).filter(Boolean))].sort(),selected=$('projectFilter').value;
    $('projectFilter').innerHTML='<option value="">Все проекты</option>'+projects.map(p=>`<option>${esc(p)}</option>`).join('');$('projectFilter').value=selected;
    $('projectNames').innerHTML=projects.map(p=>`<option value="${esc(p)}"></option>`).join('');
    renderBoard();renderCandidates();
    $('workSchedule').innerHTML=p.blocks.map(b=>`<div class="schedule-block ${esc(b.kind)}"><time>${clock.format(new Date(b.start))}–${clock.format(new Date(b.end))}</time><div><strong>${esc(b.title)}</strong><span>${b.kind==='focus'?'Предложено · не забронировано':b.kind==='break'?'Восстановление':'Календарь'}</span></div></div>`).join('')||empty('Временных блоков пока нет.');
    $('scheduleWarnings').innerHTML=p.warnings.map(w=>`<p class="schedule-warning">${esc(w)}</p>`).join('');
    $('exportFocus').href=`/api/focus.ics?date=${encodeURIComponent(p.date)}`;$('exportFocus').hidden=!p.blocks.some(b=>b.kind==='focus');
    $('energy').value=d.energyDate===p.date?d.energy:'auto';$('energyReason').textContent=`${p.mode==='gentle'?'Бережный режим: блоки до 45 минут. ':''}${p.reason}`;
    const h=d.health,stale=!h||Date.now()-new Date(h.sleepEnd).getTime()>36*3600000;
    $('healthState').innerHTML=h?`<p class="health-value">${h.sleepHours == null?'Нет записей сна':`${Number(h.sleepHours).toFixed(1)} ч сна`}</p><p class="helper">За 24 часа до экспорта ${esc(formatDateTime.format(new Date(h.measuredAt)))}${stale?' · Данные сна устарели или отсутствуют':''}</p>`:'<p class="helper">Данные сна не подключены. Можно выбрать нагрузку вручную.</p>';
    $('healthBridge').textContent=snapshot.healthBridge;
    $('weeklyTasks').innerHTML=tasks.filter(t=>t.weekly&&t.status!=='done').map(t=>taskCard(t)).join('')||empty('Отметьте приоритеты недели в карточках задач.');
    const noNext=projects.filter(p=>tasks.some(t=>t.project===p&&t.status!=='done'&&(t.weekly||['doing','waiting'].includes(t.status)))&&!tasks.some(t=>t.project===p&&['ready','doing'].includes(t.status)));
    $('projectWarnings').innerHTML=noNext.map(p=>`<p class="schedule-warning">${esc(p)}: нет подготовленного действия или задачи в работе</p>`).join('')||empty('Нет активных проектов, требующих следующего действия.');
    const attention=tasks.filter(t=>(t.status==='waiting'&&(!t.checkDate||t.checkDate<=localDate())) || (t.mitDate&&t.mitDate<localDate()&&t.status!=='done') || (t.status==='doing'&&Date.now()-new Date(t.updatedAt).getTime()>7*86400000));
    $('reviewDecisions').innerHTML=attention.length?'<h3>Требуют решения: продолжить, завершить или отложить</h3>'+attention.map(t=>taskCard(t)).join(''):empty('Нет просроченных планов и ожиданий к проверке.');
    $('lastReview').textContent=d.review.at&&!d.review.at.startsWith('0001')?`Последний обзор: ${formatDateTime.format(new Date(d.review.at))}. ${d.review.results||''}`:'Обзор ещё не проводился.';
    if(!$('settingsForm').contains(document.activeElement)) Object.entries(d.settings).forEach(([k,v])=>{if($('settingsForm').elements[k])$('settingsForm').elements[k].value=v});
  }
  $('workspace').addEventListener('click',async e=>{
    const b=e.target.closest('button');if(!b)return;
    if(b.dataset.main || b.dataset.daily){if(await mutate({action:b.dataset.main?'main':'daily',date:$('workDate').value,task:{id:b.dataset.main||b.dataset.daily}}))$('dailyPicker').open=false;}
    if(b.dataset.dismiss){await mutate({action:'dismiss',key:b.dataset.dismiss});}
    if(b.dataset.done){const t=snapshot.data.tasks.find(t=>t.id===b.dataset.done);await mutate({action:'update',task:cleanTask({...t,status:'done'})});}
    if(b.dataset.import){b.disabled=true;await mutate({action:'import',key:b.dataset.import,task:{minutes:90}});b.disabled=false;}
    if(b.dataset.edit){editing=snapshot.data.tasks.find(t=>t.id===b.dataset.edit);editingVersion=snapshot.data.version;const f=$('taskForm');['id','title','project','minutes','status','resume','waitingOn','checkDate'].forEach(k=>f.elements[k].value=editing[k]||'');f.elements.mit.checked=editing.mitDate===$('workDate').value;f.elements.weekly.checked=editing.weekly;$('mitDateLabel').textContent=$('workDate').value;$('taskError').textContent='';$('taskDialog').showModal();}
    if(b.dataset.focus){const t=snapshot.data.tasks.find(t=>t.id===b.dataset.focus);if($('workDate').value!==localDate()){feedback('Фокус-сессию можно начать только для сегодняшнего дня.');return;}if(session){feedback('Сначала завершите текущую фокус-сессию.');return;}const available=snapshot.schedule.blocks.find(b=>b.kind==='focus'&&b.taskId===t.id&&new Date(b.start).getTime()<=Date.now()+60000&&new Date(b.end).getTime()>Date.now());if(!available){feedback('Сейчас для этой задачи нет свободного блока. Проверьте расписание или измените MIT.');return;}if(await mutate({action:'update',task:cleanTask({...t,status:'doing'})})){session={title:t.title,end:new Date(available.end).getTime()};saveSession();tick();}}
  });
  function cleanTask(t){const {MITDate,...rest}=t;return rest;}
  $('chooseDaily').addEventListener('click',()=>{$('dailyPicker').open=true;$('dailyPicker').scrollIntoView({block:'nearest',behavior:'smooth'})});
  $('restoreCandidates').addEventListener('click',()=>mutate({action:'restoreCandidates'}));
  $('planTomorrow').addEventListener('click',()=>{const date=new Date(localDate()+'T12:00:00Z');date.setUTCDate(date.getUTCDate()+1);$('workDate').value=date.toISOString().slice(0,10);$('workDate').dispatchEvent(new Event('change'));$('dailyPicker').open=true;});
  $('closeTask').addEventListener('click',()=>$('taskDialog').close());
  $('taskForm').addEventListener('submit',async e=>{e.preventDefault();const f=e.target;const t={...cleanTask(editing),title:f.elements.title.value.trim(),project:f.elements.project.value.trim(),minutes:Number(f.elements.minutes.value),status:f.elements.status.value,mitDate:f.elements.mit.checked?$('workDate').value:(editing.mitDate!==$('workDate').value?editing.mitDate:''),weekly:f.elements.weekly.checked,resume:f.elements.resume.value.trim(),waitingOn:f.elements.waitingOn.value.trim(),checkDate:f.elements.checkDate.value};if(await mutate({action:'update',task:t},editingVersion))$('taskDialog').close()});
  $('captureForm').addEventListener('submit',async e=>{e.preventDefault();if(await mutate({action:'capture',task:{title:$('captureTitle').value.trim(),minutes:90}}))$('captureTitle').value=''});
  $('energy').addEventListener('change',()=>mutate({action:'energy',date:$('workDate').value,energy:$('energy').value}));
  $('settingsForm').addEventListener('submit',async e=>{e.preventDefault();const settings=Object.fromEntries(new FormData(e.target));['reserve','block','break','sleepTarget'].forEach(k=>settings[k]=Number(settings[k]));await mutate({action:'settings',settings})});
  $('reviewForm').addEventListener('submit',async e=>{e.preventDefault();if(await mutate({action:'review',results:e.target.elements.results.value}))e.target.reset()});
  $('workDate').addEventListener('change',async()=>{if($('workDate').value){try{snapshot=await request();render()}catch(e){feedback(e.message)}}});
  $('projectFilter').addEventListener('change',renderBoard);$('candidateSource').addEventListener('change',renderCandidates);
  let session;
  function readSession(){try{const value=JSON.parse(localStorage.getItem('spotter-focus-session'));session=value&&typeof value.title==='string'&&Number.isFinite(value.end)?value:null}catch{session=null}}
  readSession();window.addEventListener('storage',e=>{if(e.key==='spotter-focus-session'){readSession();tick()}});
  function saveSession(){try{if(session)localStorage.setItem('spotter-focus-session',JSON.stringify(session));else localStorage.removeItem('spotter-focus-session')}catch{}}
  function tick(){ $('focusSession').hidden=!session;if(!session)return;$('focusName').textContent=session.title;const left=Math.max(0,Math.ceil((session.end-Date.now())/1000));$('focusClock').textContent=left?`${Math.floor(left/60)}:${String(left%60).padStart(2,'0')}`:'Блок завершён. Время для перерыва.'; }
  $('stopFocus').addEventListener('click',()=>{session=null;saveSession();tick()});
  window.addEventListener('spotter:state',load);
  setInterval(tick,1000);tick();load();setInterval(load,60000);
})();
