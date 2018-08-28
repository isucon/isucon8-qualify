"use strict";

// Note: ES2015 Supported Browsers: https://kangax.github.io/compat-table/es6/

const DOM = {
  appWrapper: $('#app-wrapper'),
  confirmModal: $('#confirm-modal'),
  registerModal: $('#register-modal'),
  loginModal: $('#login-modal'),
  eventModal: $('#event-modal'),
  myPageModal: $('#my-page-modal'),
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
    if (res.status === 204) {
      return Promise.resolve({});
    }
    return res.json();
  };

  const handleJSONError = res => {
    if ('error' in res) {
      return Promise.reject(res.error);
    }
    return Promise.resolve(res);
  };

  return {
    User: {
      register (nickname, loginName, password) {
        return fetch('/api/users', {
          method: 'POST',
          headers: new Headers({ 'Content-Type': 'application/json' }),
          body: JSON.stringify({ nickname: nickname, login_name: loginName, password: password }),
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
      },
      login (loginName, password) {
        return fetch('/api/actions/login', {
          method: 'POST',
          headers: new Headers({ 'Content-Type': 'application/json' }),
          body: JSON.stringify({ login_name: loginName, password: password }),
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
      },
      logout () {
        return fetch('/api/actions/logout', {
          method: 'POST',
          headers: new Headers({ 'Content-Type': 'application/json' }),
          body: '{}',
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
      },
      getDetails (id) {
        return fetch(`/api/users/${id}`, {
          method: 'GET',
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
      },
    },
    Event: {
      getAll () {
        return fetch('/api/events', {
          method: 'GET',
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
      },
      getDetails (eventId) {
        return fetch(`/api/events/${eventId}`, {
          method: 'GET',
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
      },
      reserveSheet (eventId, sheetRank) {
        return fetch(`/api/events/${eventId}/actions/reserve`, {
          method: 'POST',
          headers: new Headers({ 'Content-Type': 'application/json' }),
          body: JSON.stringify({ sheet_rank: sheetRank }),
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
      },
      freeSheet (eventId, sheetRank, sheetNum) {
        return fetch(`/api/events/${eventId}/sheets/${sheetRank}/${sheetNum}/reservation`, {
          method: 'DELETE',
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
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
    const events = DOM.appWrapper.data('events');
    return {
      events: events,
      ranks: ['S', 'A', 'B', 'C'],
    };
  },
  methods: {
    open (eventId) { openEventModal(eventId) },
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
    reserveSheet (sheetRank) {
      const message = sheetRank+'席: '+this.event.sheets[sheetRank].price+'円を予約購入します。よろしいですか？';
      confirm('席の予約', message).then(() => {
        return showWaitingDialog('Processing...');
      }).then(() => {
        return API.Event.reserveSheet(this.event.id, sheetRank);
      }).then(result => {
        const sheet = this.event.sheets[sheetRank].detail[result.sheet_num-1];
        sheet.reserved = true;
        sheet.mine = true;
        this.event.sheets[sheetRank].remains--;
        this.event.remains--;
        this.$forceUpdate();
      }).catch(showError).finally(hideWaitingDialog);
    },
    freeSheet (sheetRank, sheetNum) {
      const sheet = this.event.sheets[sheetRank].detail[sheetNum-1];
      if (!sheet.mine) return;

      const message = '予約をキャンセルしますか？: '+sheetRank+'-'+sheet.num;
      confirm('予約のキャンセル', message).then(() => {
        return showWaitingDialog('Processing...');
      }).then(() => {
        return API.Event.freeSheet(this.event.id, sheetRank, sheetNum);
      }).then(() => {
        sheet.reserved = false;
        sheet.mine = false;
        this.event.sheets[sheetRank].remains++;
        this.event.remains++;
        this.$forceUpdate();
      }).catch(showError).finally(hideWaitingDialog);
    },
  },
});

function updateEventModal(eventId) {
  return new Promise((resolve, reject) => {
    API.Event.getDetails(eventId).then(event => {
      EventModal.$data.event = event;
      resolve(event);
    }).catch(reject);
  });
}

function openEventModal(eventId) {
  const id = setInterval(() => {
    updateEventModal(eventId).then(event => {
      EventList.$data.events.forEach((e, i, events) => {
        if (e.id !== event.id) return;
        events[i] = event;
      });
      EventList.$forceUpdate();
    });
  }, 10000);

  showWaitingDialog().then(() => updateEventModal(eventId)).then(() =>{
    DOM.eventModal.modal('show');
    DOM.eventModal.one('hide.bs.modal', () => clearInterval(id));
  }).catch(err => {
    showError(err);
    clearInterval(id);
  }).finally(hideWaitingDialog);
}

const MyPageModal = new Vue({
  el: '#my-page-modal .modal-dialog',
  data () {
    return {
      user: {
        nickname: '',
        total_price: '',
        recent_events: [],
        recent_reservations: [],
      },
      ranks: ['S', 'A', 'B', 'C'],
    };
  },
  methods: {
    openEvent (event) {
      if (!event.public || event.closed) {
        return;
      }
      openEventModal(event.id);
      DOM.myPageModal.modal('hide');
    },
    formatDateTime(epoch) {
      const dt = new Date(epoch * 1000);
      return dt.toLocaleString();
    },
  },
});

function updateMyPageModal(userId) {
  return new Promise((resolve, reject) => {
    API.User.getDetails(userId).then(user => {
      MyPageModal.$data.user = user;
      resolve(user);
    }).catch(reject);
  });
}

function openMyPageModal(userId) {
  showWaitingDialog().then(() => updateMyPageModal(userId)).then(() => DOM.myPageModal.modal('show')).catch(showError).finally(hideWaitingDialog);
}

const MenuBar = new Vue({
  el: '#menu-bar',
  data () {
    const currentUser = DOM.appWrapper.data('login-user');
    return {
      currentUser: currentUser,
    };
  },
  methods: {
    signUp () {
      DOM.registerModal.modal('show');
    },
    signIn () {
      DOM.loginModal.modal('show');
    },
    signOut () {
      confirm('サインアウト', '本当にサインアウトしますか?').then(() => {
        return API.User.logout();
      }).then(() => {
        this.currentUser = null;
      }).catch(showError);
    },
    showMyPage() {
      openMyPageModal(this.currentUser.id);
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
      showWaitingDialog('Processing...').then(() => {
        return API.User.login(this.loginName, this.password);
      }).then(user => {
        MenuBar.$data.currentUser = user;
        DOM.loginModal.modal('hide');
      }).catch(showError).finally(hideWaitingDialog);
    },
  },
});

new Vue({
  el: '#register-modal .modal-dialog',
  data () {
    return {
      nickname: '',
      loginName: '',
      password: '',
    };
  },
  methods: {
    submit () {
      const loginName = this.loginName;
      const password = this.password;
      showWaitingDialog('Processing...').then(() => {
        return API.User.register(this.nickname, loginName, password);
      }).then(() => {
        return API.User.login(loginName, password);
      }).then(user => {
        MenuBar.$data.currentUser = user;
        DOM.registerModal.modal('hide');
      }).catch(showError).finally(hideWaitingDialog);
    },
  },
});

$('body').on('shown.bs.modal', '.modal', e => {
  $('input', e.target).first().focus();
});
