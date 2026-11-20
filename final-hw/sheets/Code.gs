const SHEET_TX = 'Транзакции';
const SHEET_BUDGETS = 'Бюджеты';
const SHEET_REPORT = 'Отчёт';
const SHEET_EXPORT = 'Выгрузка';

const TX_HEADER = ['Дата', 'Сумма', 'Категория', 'Описание', 'Статус', 'ID'];
const BUDGET_HEADER = ['Категория', 'Лимит', 'Статус'];

function onOpen() {
  SpreadsheetApp.getUi()
    .createMenu('CashCraft')
    .addItem('Настройки подключения…', 'showSettings')
    .addItem('Создать листы', 'setupSheets')
    .addSeparator()
    .addItem('Отправить новые транзакции', 'sendTransactions')
    .addItem('Отправить бюджеты', 'sendBudgets')
    .addSeparator()
    .addItem('Загрузить отчёт за период', 'loadReport')
    .addItem('Загрузить все транзакции', 'loadTransactions')
    .addSeparator()
    .addItem('Включить автоотправку при вводе', 'enableAutoSend')
    .addItem('Выключить автоотправку', 'disableAutoSend')
    .addToUi();
}

// ---------- настройки и авторизация ----------

function props() {
  return PropertiesService.getUserProperties();
}

function showSettings() {
  const ui = SpreadsheetApp.getUi();
  const p = props();

  const url = ui.prompt('Адрес API', 'Публичный адрес gateway, например https://xxxx.trycloudflare.com\nСейчас: ' + (p.getProperty('API_URL') || 'не задан'), ui.ButtonSet.OK_CANCEL);
  if (url.getSelectedButton() !== ui.Button.OK) return;
  const login = ui.prompt('Логин', 'Логин пользователя CashCraft', ui.ButtonSet.OK_CANCEL);
  if (login.getSelectedButton() !== ui.Button.OK) return;
  const password = ui.prompt('Пароль', 'Пароль (хранится в свойствах пользователя скрипта)', ui.ButtonSet.OK_CANCEL);
  if (password.getSelectedButton() !== ui.Button.OK) return;

  p.setProperties({
    API_URL: url.getResponseText().trim().replace(/\/+$/, ''),
    LOGIN: login.getResponseText().trim(),
    PASSWORD: password.getResponseText(),
  });
  p.deleteProperty('TOKEN');
  p.deleteProperty('TOKEN_EXP');

  getToken(true);
  ui.alert('Подключение настроено, вход выполнен.');
}

function apiUrl() {
  const url = props().getProperty('API_URL');
  if (!url) throw new Error('Не задан адрес API: CashCraft → Настройки подключения');
  return url;
}

function getToken(force) {
  const p = props();
  const token = p.getProperty('TOKEN');
  const exp = Number(p.getProperty('TOKEN_EXP') || 0);
  if (!force && token && Date.now() < exp - 60 * 1000) return token;

  const resp = UrlFetchApp.fetch(apiUrl() + '/api/auth/login', {
    method: 'post',
    contentType: 'application/json',
    payload: JSON.stringify({ login: p.getProperty('LOGIN'), password: p.getProperty('PASSWORD') }),
    muteHttpExceptions: true,
  });
  const body = JSON.parse(resp.getContentText() || '{}');
  if (resp.getResponseCode() !== 200) {
    throw new Error('Не удалось войти: ' + (body.error || resp.getResponseCode()));
  }
  p.setProperties({ TOKEN: body.token, TOKEN_EXP: String(new Date(body.expires_at).getTime()) });
  return body.token;
}

function api(method, path, payload) {
  const send = (token) => UrlFetchApp.fetch(apiUrl() + path, {
    method: method,
    contentType: 'application/json',
    headers: { Authorization: 'Bearer ' + token },
    payload: payload === undefined ? undefined : JSON.stringify(payload),
    muteHttpExceptions: true,
  });

  let resp = send(getToken(false));
  if (resp.getResponseCode() === 401) {
    resp = send(getToken(true));
  }
  const text = resp.getContentText();
  let body = null;
  try {
    body = JSON.parse(text);
  } catch (e) {
    body = { error: text };
  }
  return { code: resp.getResponseCode(), body: body };
}

// ---------- листы ----------

function sheet(name, header) {
  const ss = SpreadsheetApp.getActive();
  let sh = ss.getSheetByName(name);
  if (!sh) sh = ss.insertSheet(name);
  if (header && sh.getLastRow() === 0) {
    sh.appendRow(header);
    sh.getRange(1, 1, 1, header.length).setFontWeight('bold');
    sh.setFrozenRows(1);
  }
  return sh;
}

function setupSheets() {
  sheet(SHEET_TX, TX_HEADER).getRange('A:A').setNumberFormat('yyyy-mm-dd');
  sheet(SHEET_BUDGETS, BUDGET_HEADER);
  const rep = sheet(SHEET_REPORT);
  if (rep.getLastRow() === 0) {
    rep.getRange('A1:B2').setValues([['С', '2025-09-01'], ['По', '2025-09-30']]);
    rep.getRange('B1:B2').setNumberFormat('@');
  }
  sheet(SHEET_EXPORT);
}

function formatDate(v) {
  if (Object.prototype.toString.call(v) === '[object Date]') {
    return Utilities.formatDate(v, SpreadsheetApp.getActive().getSpreadsheetTimeZone(), 'yyyy-MM-dd');
  }
  return String(v).trim();
}

function toNumber(v) {
  if (typeof v === 'number') return v;
  return Number(String(v).replace(/\s/g, '').replace(',', '.'));
}

function rowToTransaction(row) {
  return {
    date: formatDate(row[0]),
    amount: toNumber(row[1]),
    category: String(row[2]).trim(),
    description: String(row[3] || '').trim(),
  };
}

function isReady(row) {
  return row[0] !== '' && row[1] !== '' && row[2] !== '' && row[4] === '';
}

// ---------- запись данных ----------

function sendTransactions() {
  const sh = sheet(SHEET_TX, TX_HEADER);
  if (sh.getLastRow() < 2) return;
  const rows = sh.getRange(2, 1, sh.getLastRow() - 1, TX_HEADER.length).getValues();

  const pending = [];
  rows.forEach((row, i) => {
    if (isReady(row)) pending.push({ rowIndex: i + 2, tx: rowToTransaction(row) });
  });
  if (pending.length === 0) {
    SpreadsheetApp.getActive().toast('Новых транзакций нет');
    return;
  }

  const res = api('post', '/api/transactions/bulk?workers=4', pending.map((p) => p.tx));
  if (res.code !== 200) {
    throw new Error('Ошибка импорта: ' + (res.body.error || res.code));
  }

  const failed = {};
  res.body.errors.forEach((e) => { failed[e.index] = e.error; });
  pending.forEach((p, i) => {
    const status = failed[i] ? '✗ ' + failed[i] : '✓ сохранено';
    sh.getRange(p.rowIndex, 5).setValue(status);
  });
  SpreadsheetApp.getActive().toast('Принято: ' + res.body.accepted + ', отклонено: ' + res.body.rejected);
}

function sendRow(sh, rowIndex) {
  const row = sh.getRange(rowIndex, 1, 1, TX_HEADER.length).getValues()[0];
  if (!isReady(row)) return;
  const res = api('post', '/api/transactions', rowToTransaction(row));
  if (res.code === 201) {
    sh.getRange(rowIndex, 5, 1, 2).setValues([['✓ сохранено', res.body.id]]);
  } else {
    sh.getRange(rowIndex, 5).setValue('✗ ' + (res.body.error || res.code));
  }
}

function sendBudgets() {
  const sh = sheet(SHEET_BUDGETS, BUDGET_HEADER);
  if (sh.getLastRow() < 2) return;
  const rows = sh.getRange(2, 1, sh.getLastRow() - 1, BUDGET_HEADER.length).getValues();
  let ok = 0;
  rows.forEach((row, i) => {
    if (row[0] === '' || row[1] === '') return;
    const res = api('post', '/api/budgets', { category: String(row[0]).trim(), limit: toNumber(row[1]) });
    sh.getRange(i + 2, 3).setValue(res.code === 201 ? '✓ сохранено' : '✗ ' + (res.body.error || res.code));
    if (res.code === 201) ok++;
  });
  SpreadsheetApp.getActive().toast('Бюджетов сохранено: ' + ok);
}

// ---------- отчёты ----------

function loadReport() {
  const sh = sheet(SHEET_REPORT);
  const from = formatDate(sh.getRange('B1').getValue());
  const to = formatDate(sh.getRange('B2').getValue());

  const res = api('get', '/api/reports/summary?from=' + encodeURIComponent(from) + '&to=' + encodeURIComponent(to));
  if (res.code !== 200) {
    throw new Error('Ошибка отчёта: ' + (res.body.error || res.code));
  }

  sh.getRange(4, 1, Math.max(sh.getMaxRows() - 3, 1), 4).clearContent();
  const table = [['Категория', 'Потрачено', 'Бюджет', '% бюджета']];
  res.body.categories.forEach((c) => {
    table.push([
      c.category,
      c.total,
      c.budget === undefined ? '' : c.budget,
      c.budget_percent === undefined ? '' : c.budget_percent / 100,
    ]);
  });
  table.push(['Итого', res.body.total, '', '']);

  sh.getRange(4, 1, table.length, 4).setValues(table);
  sh.getRange(4, 1, 1, 4).setFontWeight('bold');
  sh.getRange(4 + table.length - 1, 1, 1, 2).setFontWeight('bold');
  sh.getRange(5, 2, table.length - 1, 2).setNumberFormat('#,##0.00');
  sh.getRange(5, 4, table.length - 1, 1).setNumberFormat('0.00%');
  SpreadsheetApp.getActive().toast('Отчёт за ' + from + ' — ' + to + ' загружен');
}

function loadTransactions() {
  const res = api('get', '/api/transactions');
  if (res.code !== 200) {
    throw new Error('Ошибка загрузки: ' + (res.body.error || res.code));
  }
  const sh = sheet(SHEET_EXPORT);
  sh.clearContents();
  const table = [['ID', 'Дата', 'Сумма', 'Категория', 'Описание']];
  res.body.forEach((t) => table.push([t.id, t.date, t.amount, t.category, t.description]));
  sh.getRange(1, 1, table.length, 5).setValues(table);
  sh.getRange(1, 1, 1, 5).setFontWeight('bold');
  SpreadsheetApp.getActive().toast('Загружено транзакций: ' + (table.length - 1));
}

// ---------- автоотправка ----------

function enableAutoSend() {
  disableAutoSend();
  ScriptApp.newTrigger('onEditSend').forSpreadsheet(SpreadsheetApp.getActive()).onEdit().create();
  SpreadsheetApp.getActive().toast('Автоотправка включена: строка уходит в API, когда заполнены дата, сумма и категория');
}

function disableAutoSend() {
  ScriptApp.getProjectTriggers()
    .filter((t) => t.getHandlerFunction() === 'onEditSend')
    .forEach((t) => ScriptApp.deleteTrigger(t));
}

function onEditSend(e) {
  const sh = e.range.getSheet();
  if (sh.getName() !== SHEET_TX || e.range.getRow() < 2 || e.range.getColumn() > 4) return;
  for (let r = e.range.getRow(); r <= e.range.getLastRow(); r++) {
    sendRow(sh, r);
  }
}
