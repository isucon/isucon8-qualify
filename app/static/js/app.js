var API = {};

API.User = {
  register: function (nickname, loginName, password) {
    return fetch('/api/users', {
      method: 'POST',
      headers: new Headers({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ nickname: nickname, login_name: loginName, password: password }),
      credentials: 'same-origin',
    }).then(function (res) {
      return res.json();
    });
  },
  login: function (loginName, password) {
    return fetch('/api/actions/login', {
      method: 'POST',
      headers: new Headers({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ login_name: loginName, password: password }),
      credentials: 'same-origin',
    }).then(function (res) {
      return res.json();
    });
  },
  logout: function () {
    return fetch('/api/actions/logout', {
      method: 'POST',
      headers: new Headers({ 'Content-Type': 'application/json' }),
      body: '{}',
      credentials: 'same-origin',
    });
  },
};

API.Event = {
  getAll: function () {
    return fetch('/api/events', {
      method: 'GET',
      credentials: 'same-origin',
    }).then(function (res) {
      return res.json();
    });
  },
  getDetails: function (eventId) {
    return fetch('/api/events/'+eventId, {
      method: 'GET',
      credentials: 'same-origin',
    }).then(function (res) {
      return res.json();
    });
  },
  reserveSheet: function (eventId, sheetRank) {
    return fetch('/api/events/'+eventId+'/actions/reserve', {
      method: 'POST',
      headers: new Headers({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ sheet_rank: sheetRank }),
      credentials: 'same-origin',
    }).then(function (res) {
      return res.json();
    });
  },
  freeSheet: function (eventId, sheetRank, sheetNum) {
    return fetch('/api/events/'+eventId+'/sheets/'+sheetRank+'/'+sheetNum+'/reservation', {
      method: 'DELETE',
      credentials: 'same-origin',
    });
  },
};

$(function () {
  console.log('app.js is loaded!');
});
