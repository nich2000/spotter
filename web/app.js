const formatDateTime = new Intl.DateTimeFormat('ru-RU', {
  dateStyle: 'medium',
  timeStyle: 'short',
});
const formatTime = new Intl.DateTimeFormat('ru-RU', {
  hour: '2-digit',
  minute: '2-digit',
});

const empty = (text) => `<div class="empty">${escapeHTML(text)}</div>`;

function escapeHTML(value) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;');
}

function parseDate(value) {
  if (!value) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

function renderList(id, items, renderer, emptyText) {
  const node = document.getElementById(id);
  node.innerHTML = items?.length ? items.map(renderer).join('') : empty(emptyText);
}

function renderState(state) {
  const now = new Date();
  document.getElementById('today').textContent = new Intl.DateTimeFormat('ru-RU', {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  }).format(now);

  const generated = parseDate(state.generatedAt);
  document.getElementById('updatedAt').textContent = generated
    ? `Обновлено ${formatDateTime.format(generated)}`
    : 'Ещё не обновлялось';

  document.getElementById('planSummary').textContent = state.plan?.summary || 'План пока не сформирован.';
  renderBullets('planBlocks', state.plan?.blocks, 'Нет временных блоков');
  renderBullets('planFocus', state.plan?.focus, 'Нет фокуса');
  renderBullets('planRisks', state.plan?.risks, 'Нет рисков');

  renderList('calendar', state.calendar, renderCalendar, 'Событий нет');
  renderList('reminders', state.reminders, renderReminder, 'Активных напоминаний нет');
  renderList('mail', state.mail, renderMail, 'Непрочитанных писем нет');
  renderList('notes', state.notes, renderNote, 'Заметок нет');
  renderCounts(state);
  renderSources(state.sources || []);
}

function renderCounts(state) {
  document.getElementById('calendarCount').textContent = countLabel(state.calendar?.length || 0, 'событие', 'события', 'событий');
  document.getElementById('remindersCount').textContent = countLabel(state.reminders?.length || 0, 'напоминание', 'напоминания', 'напоминаний');
  document.getElementById('mailCount').textContent = countLabel(state.mail?.length || 0, 'письмо', 'письма', 'писем');
  document.getElementById('notesCount').textContent = countLabel(state.notes?.length || 0, 'заметка', 'заметки', 'заметок');
}

function countLabel(count, one, few, many) {
  const mod10 = count % 10;
  const mod100 = count % 100;
  let word = many;
  if (mod10 === 1 && mod100 !== 11) word = one;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) word = few;
  return `${count} ${word}`;
}

function renderBullets(id, items, emptyText) {
  const node = document.getElementById(id);
  node.innerHTML = items?.length
    ? items.map((item) => `<li>${escapeHTML(item)}</li>`).join('')
    : `<li class="muted">${escapeHTML(emptyText)}</li>`;
}

function renderCalendar(item) {
  const start = parseDate(item.start);
  const end = parseDate(item.end);
  const time = start && end ? `${formatTime.format(start)}-${formatTime.format(end)}` : '';
  return `<article class="item">
    <div class="item-main">
      <strong>${escapeHTML(item.title)}</strong>
      <span>${escapeHTML(time)}</span>
    </div>
    <p>${escapeHTML([item.calendar, item.location].filter(Boolean).join(' · '))}</p>
  </article>`;
}

function renderReminder(item) {
  const due = parseDate(item.dueDate);
  return `<article class="item">
    <div class="item-main">
      <strong>${escapeHTML(item.title)}</strong>
      <span>${due ? formatDateTime.format(due) : 'Без срока'}</span>
    </div>
    <p>${escapeHTML([item.list, item.priority ? `P${item.priority}` : ''].filter(Boolean).join(' · '))}</p>
  </article>`;
}

function renderMail(item) {
  const date = parseDate(item.date);
  const unread = item.isUnread ? '<span class="badge">unread</span>' : '';
  return `<article class="item">
    <div class="item-main">
      <strong>${escapeHTML(item.subject || '(без темы)')} ${unread}</strong>
      <span>${date ? formatDateTime.format(date) : ''}</span>
    </div>
    <p>${escapeHTML(item.sender)}${item.preview ? ` · ${escapeHTML(item.preview)}` : ''}</p>
  </article>`;
}

function renderNote(item) {
  const updated = parseDate(item.updatedAt);
  return `<article class="item">
    <div class="item-main">
      <strong>${escapeHTML(item.title)}</strong>
      <span>${updated ? formatDateTime.format(updated) : ''}</span>
    </div>
    <p>${escapeHTML(item.bodyPreview)}</p>
  </article>`;
}

function renderSources(sources) {
  const node = document.getElementById('sources');
  node.innerHTML = sources.length ? sources.map((source) => {
    const updated = parseDate(source.updatedAt);
    return `<div class="source ${source.ok ? 'ok' : 'fail'}">
      <div><strong>${escapeHTML(source.name)}</strong><span>${source.ok ? 'OK' : 'ERROR'}</span></div>
      <p>${escapeHTML(source.error || (updated ? formatDateTime.format(updated) : ''))}</p>
    </div>`;
  }).join('') : empty('Статусы источников пока недоступны');
}

async function loadState() {
  const response = await fetch('/api/state');
  if (!response.ok) throw new Error(`state request failed: ${response.status}`);
  renderState(await response.json());
}

document.getElementById('refresh').addEventListener('click', async () => {
  const button = document.getElementById('refresh');
  const originalText = button.textContent;
  button.disabled = true;
  button.textContent = 'Обновляю источники и план...';
  try {
    const response = await fetch('/api/refresh', { method: 'POST' });
    if (!response.ok) throw new Error(`refresh request failed: ${response.status}`);
    renderState(await response.json());
  } finally {
    button.textContent = originalText;
    button.disabled = false;
  }
});

document.querySelectorAll('[data-collapse-target]').forEach((button) => {
  if (!button.hasAttribute('aria-expanded')) {
    button.setAttribute('aria-expanded', 'false');
  }
  button.addEventListener('click', () => {
    const target = document.getElementById(button.dataset.collapseTarget);
    const expanded = button.getAttribute('aria-expanded') === 'true';
    button.setAttribute('aria-expanded', String(!expanded));
    target.hidden = expanded;
  });
});

loadState().catch(console.error);

const events = new EventSource('/events');
events.addEventListener('open', () => {
  document.getElementById('connection').textContent = 'SSE: online';
});
events.addEventListener('error', () => {
  document.getElementById('connection').textContent = 'SSE: reconnecting';
});
events.addEventListener('update', (event) => {
  renderState(JSON.parse(event.data));
});
