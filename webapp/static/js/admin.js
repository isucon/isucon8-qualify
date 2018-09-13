"use strict";

// Note: ES2015 Supported Browsers: https://kangax.github.io/compat-table/es6/

const DOM = {
  appWrapper: $('#app-wrapper'),
  confirmModal: $('#confirm-modal'),
  loginModal: $('#login-modal'),
  eventModal: $('#event-modal'),
  eventRegistrationModal: $('#event-registration-modal'),
};

const Errors = {
  login_required:        'ログインしてください',
  duplicated:            'すでに登録済です',
  forbidden:             '権限がありません',
  authentication_failed: '認証に失敗しました',
  not_found:             '存在しません',
  invalid_rank:          'そのランクを指定することはできません',
  invalid_event:         'そのイベントを指定することはできません',
  invalid_sheet:         'そのシートを指定することはできません',
  not_reserved:          'その席は予約されていません',
  not_permitted:         'その操作はできません',
  unwknown:              '不明なエラーです',
};

function showError(err) {
  const msg = Errors[err];
  setTimeout(() => {
    alert(msg || err);
  }, 300);
}

function showWaitingDialog(msg) {
  return new Promise((resolve, reject) => {
    waitingDialog.show(msg || 'Loading...');
    setTimeout(() => {
      resolve();
    }, 300);
  });
}

function hideWaitingDialog() {
  waitingDialog.hide();
}

const API = (() => {
  const handleJSON = res => {
    return res.json();
  };

  const handleJSONError = res => {
    if ('error' in res) {
      return Promise.reject(res.error);
    }
    return Promise.resolve(res);
  };

  return {
    Administrator: {
      login (loginName, password) {
        return fetch('/admin/api/actions/login', {
          method: 'POST',
          headers: new Headers({ 'Content-Type': 'application/json' }),
          body: JSON.stringify({ login_name: loginName, password: password }),
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
      },
      logout () {
        return fetch('/admin/api/actions/logout', {
          method: 'POST',
          headers: new Headers({ 'Content-Type': 'application/json' }),
          body: '{}',
          credentials: 'same-origin',
        });
      },
    },
    Event: {
      register (title, price, isPublic) {
        return fetch('/admin/api/events', {
          method: 'POST',
          headers: new Headers({ 'Content-Type': 'application/json' }),
          body: JSON.stringify({ title, price, public: isPublic }),
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
      },
      edit (eventId, isPublic, isClosed) {
        return fetch(`/admin/api/events/${eventId}/actions/edit`, {
          method: 'POST',
          headers: new Headers({ 'Content-Type': 'application/json' }),
          body: JSON.stringify({ public: isPublic, closed: isClosed }),
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
      },
      getAll () {
        return fetch('/admin/api/events', {
          method: 'GET',
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
      },
      getDetails (eventId) {
        return fetch('/admin/api/events/'+eventId, {
          method: 'GET',
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
      },
    },
    Report: {
      getEventSales (eventId) {
        window.open(`/admin/api/reports/events/${eventId}/sales`);
      },
      getTotalSales () {
        window.open('/admin/api/reports/sales');
      },
    },
  };
})();

const ConfirmModal = new Vue({
  el: '#confirm-modal .modal-dialog',
  data: {
    title: '',
    message: '',
    callback: null,
  },
  methods: {
    ok () {
      if (this.callback === null) return;

      this.callback();
      this.callback = null;
      DOM.confirmModal.modal('hide');
    },
  },
});

function confirm(title, message) {
  return new Promise((resolve, reject) => {
    ConfirmModal.$data.title = title;
    ConfirmModal.$data.message = message;
    ConfirmModal.$data.callback = resolve;
    DOM.confirmModal.modal('show');
  });
}

const EventList = new Vue({
  el: '.events',
  data () {
    const currentAdministrator = DOM.appWrapper.data('administrator');
    const events = DOM.appWrapper.data('events');
    return {
      events: events,
      ranks: ['S', 'A', 'B', 'C'],
      isAdmin: currentAdministrator !== null,
    };
  },
  methods: {
    open (eventId) { openEventModal(eventId) },
    openEventRegistrationModal() {
      DOM.eventRegistrationModal.modal('show');
    },
  },
});

const EventModal = new Vue({
  el: '#event-modal .modal-dialog',
  data () {
    return {
      event: { sheets: { S:{}, A:{}, B:{}, C:{} } },
      ranks: ['S', 'A', 'B', 'C'],
    };
  },
  methods: {
    divRange (n ,d) {
      const max = Math.floor(n / d);
      const range = [];
      for (let i = 1; i <= max; i++) {
        range.push(i);
      }
      return range;
    },
    isSoldOut (sheetRank) {
      return this.event.sheets[sheetRank].remains === 0;
    },
    publish() {
      const message = 'このイベントを公開しますか？';
      confirm('イベントの公開', message).then(() => {
        return API.Event.edit(this.event.id, true, false);
      }).then((event) => {
        this.event = event;
      }).catch(err => {
        showError(err);
      });
    },
    close() {
      const message = 'このイベントを終了しますか？ (戻すことはできません)';
      confirm('イベントの終了', message).then(() => {
        return API.Event.edit(this.event.id, false, true);
      }).then((event) => {
        this.event = event;
      }).catch(err => {
        showError(err);
      });
    },
    disappear() {
      const message = 'このイベントの公開を停止しますか？';
      confirm('イベントの公開停止', message).then(() => {
        return API.Event.edit(this.event.id, false, false);
      }).then((event) => {
        this.event = event;
      }).catch(err => {
        showError(err);
      });
    },
    downloadSalesReport() {
      API.Report.getEventSales(this.event.id);
    },
  },
});

function updateEventModal(eventId) {
  return new Promise((resolve, reject) => {
    API.Event.getDetails(eventId).then(event => {
      EventModal.$data.event = event;
      resolve(event);
    }).catch(err => {
      showError(err);
      reject(err);
    });
  });
}

function openEventModal(eventId) {
  updateEventModal(eventId).then(() => DOM.eventModal.modal('show'));
}

const MenuBar = new Vue({
  el: '#menu-bar',
  data () {
    const currentAdministrator = DOM.appWrapper.data('administrator');
    return {
      currentAdministrator: currentAdministrator,
    };
  },
  methods: {
    signIn () {
      DOM.loginModal.modal('show');
    },
    signOut () {
      confirm('サインアウト', '本当にサインアウトしますか？').then(() => {
        return API.Administrator.logout();
      }).then(() => {
        this.currentAdministrator = null;
        EventList.$data.isAdmin = false;
        EventList.$data.events = [];
      });
    },
    downloadSalesReport () {
      API.Report.getTotalSales();
    },
  },
});

new Vue({
  el: '#login-modal .modal-dialog',
  data () {
    return {
      loginName: '',
      password: '',
    };
  },
  methods: {
    submit () {
      API.Administrator.login(this.loginName, this.password).then(user => {
        MenuBar.$data.currentAdministrator = user;
        DOM.loginModal.modal('hide');
        return showWaitingDialog();
      }).then(() => {
        return API.Event.getAll();
      }).then(events => {
        hideWaitingDialog();
        EventList.$data.isAdmin = true;
        EventList.$data.events = events;
      }).catch(err => {
        hideWaitingDialog();
        showError(err);
      });
    },
  },
});

new Vue({
  el: '#event-registration-modal .modal-dialog',
  data () {
    return {
      title: '',
      price: 1000,
      public: 0,
    };
  },
  methods: {
    submit () {
      API.Event.register(this.title, this.price, this.public != 0).then(event => {
        EventList.$data.events.push(event);
        DOM.eventRegistrationModal.modal('hide');
      }).catch(err => {
        showError(err);
      });
    },
  },
});

$('body').on('shown.bs.modal', '.modal', e => {
  $('input', e.target).first().focus();
});
