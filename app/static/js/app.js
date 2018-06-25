var DOM = {
  confirmModal: $('#confirm-modal'),
  registerModal: $('#register-modal'),
  loginModal: $('#login-modal'),
  eventModal: $('#event-modal'),
};

function showError(err) {
  setTimeout(function () {
    alert(err);
  }, 1);
}

var API = (function () {
  var handleJSON = function (res) {
    return res.json();
  };

  var handleJSONError = function (res) {
    if ('error' in res) {
      return Promise.reject(res.error);
    }
    return Promise.resolve(res);
  };

  var User = {
    register: function (nickname, loginName, password) {
      return fetch('/api/users', {
        method: 'POST',
        headers: new Headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ nickname: nickname, login_name: loginName, password: password }),
        credentials: 'same-origin',
      }).then(handleJSON).then(handleJSONError);
    },
    login: function (loginName, password) {
      return fetch('/api/actions/login', {
        method: 'POST',
        headers: new Headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ login_name: loginName, password: password }),
        credentials: 'same-origin',
      }).then(handleJSON).then(handleJSONError);
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

  var Event = {
    getAll: function () {
      return fetch('/api/events', {
        method: 'GET',
        credentials: 'same-origin',
      }).then(handleJSON).then(handleJSONError);
    },
    getDetails: function (eventId) {
      return fetch('/api/events/'+eventId, {
        method: 'GET',
        credentials: 'same-origin',
      }).then(handleJSON).then(handleJSONError);
    },
    reserveSheet: function (eventId, sheetRank) {
      return fetch('/api/events/'+eventId+'/actions/reserve', {
        method: 'POST',
        headers: new Headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ sheet_rank: sheetRank }),
        credentials: 'same-origin',
      }).then(handleJSON).then(handleJSONError);
    },
    freeSheet: function (eventId, sheetRank, sheetNum) {
      return fetch('/api/events/'+eventId+'/sheets/'+sheetRank+'/'+sheetNum+'/reservation', {
        method: 'DELETE',
        credentials: 'same-origin',
      });
    },
  };

  return {
    User: User,
    Event: Event,
  };
})();

var ConfirmModal = new Vue({
  el: '#confirm-modal .modal-dialog',
  data: {
    title: '',
    message: '',
    callback: null,
  },
  methods: {
    ok: function () {
      if (this.callback === null) return;

      this.callback();
      this.callback = null;
      DOM.confirmModal.modal('hide');
    },
  },
});

function confirm(title, message, callback) {
   ConfirmModal.$data.title = title;
   ConfirmModal.$data.message = message;
   ConfirmModal.$data.callback = callback;
   DOM.confirmModal.modal('show');
}

var EventList = new Vue({
  el: '.events',
  data: {
    events: Torb.events,
    ranks: ['S', 'A', 'B', 'C'],
  },
  methods: {
    open: function (eventId) { openEventModal(eventId) },
  },
});

var EventModal = new Vue({
  el: '#event-modal .modal-dialog',
  data: {
    event: { sheets: { S:{}, A:{}, B:{}, C:{} } },
    ranks: ['S', 'A', 'B', 'C'],
  },
  methods: {
    divRange: function (n ,d) {
      var max = Math.floor(n / d);
      var range = [];
      for (var i = 1; i <= max; i++) {
        range.push(i);
      }
      return range;
    },
    isSoldOut: function (sheetRank) {
      return this.event.sheets[sheetRank].remains === 0;
    },
    reserveSheet: function (sheetRank) {
      var eventId = this.event.id;
      var message = sheetRank+': '+this.event.sheets[sheetRank].price+'円';
      confirm('Sheet Reservation', message, function () {
        API.Event.reserveSheet(eventId, sheetRank).then(function (result) {
          console.log(result);
          updateEventModal(eventId, function (event) {
            var events = EventList.$data.events;
            for (var i = 0, l = events.length; i < l; i++) {
              if (events[i].id !== event.id) continue;
              events[i] = event;
              break;
            }
            EventList.$forceUpdate();
          });
        }).catch(function (err) {
          showError(err);
        });
      });
    },
    freeSheet: function (sheetRank, sheetNum) {
      var sheet = this.event.sheets[sheetRank].detail[sheetNum-1];
      if (!sheet.mine) return;

      var eventId = this.event.id;
      var message = 'Do you cancel the sheet reservation?: '+sheetRank+'-'+sheet.num;
      confirm('Cancel Sheet Reservation', message, function () {
        API.Event.freeSheet(eventId, sheetRank, sheetNum).then(function () {
          updateEventModal(eventId, function (event) {
            var events = EventList.$data.events;
            for (var i = 0, l = events.length; i < l; i++) {
              if (events[i].id !== event.id) continue;
              events[i] = event;
              break;
            }
            EventList.$forceUpdate();
          });
        }).catch(function (err) {
          showError(err);
        });
      });
    },
  },
});

function updateEventModal(eventId, callback) {
  API.Event.getDetails(eventId).then(function (event) {
    EventModal.$data.event = event;
    callback(event);
  }).catch(function (err) {
    showError(err);
  });
}

function openEventModal(eventId) {
  updateEventModal(eventId, function () {
    DOM.eventModal.modal('show');
  });
}

new Vue({
  el: '#login-modal .modal-dialog',
  data: {
    loginName: '',
    password: '',
  },
  methods: {
    submit: function () {
      API.User.login(this.loginName, this.password).then(function (user) {
        Torb.currentUser = user;
        DOM.loginModal.modal('hide');
      }).catch(function (err) {
        showError(err);
      });;
    },
  },
});

new Vue({
  el: '#register-modal .modal-dialog',
  data: {
    nickname: '',
    loginName: '',
    password: '',
  },
  methods: {
    submit: function () {
      var loginName = this.loginName;
      var password = this.password;
      API.User.register(this.nickname, loginName, password).then(function () {
        return API.User.login(loginName, password);
      }).then(function (user) {
        Torb.currentUser = user;
        DOM.registerModal.modal('hide');
      }).catch(function (err) {
        showError(err);
      });
    },
  },
});

var MenuBar = new Vue({
  el: '#menu-bar',
  data: {
    currentUser: Torb.loginUser,
  },
  methods: {
    signUp: function () {
      DOM.registerModal.modal('show');
    },
    signIn: function () {
      DOM.loginModal.modal('show');
    },
    signOut: function () {
      confirm('Sign Out?', 'Do you really sign out?', function () {
        API.User.logout().then(function () {
          Torb.currentUser = null;
        });
      });
    },
  },
});

// XXX
Object.defineProperty(Torb, 'currentUser', {
  get: function() { return this.loginUser },
  set: function(user) { this.loginUser = user; MenuBar.$data.currentUser = user },
});

$('body').on('shown.bs.modal', '.modal', function (e) {
  $('input', e.target).first().focus();
});
