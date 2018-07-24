"use strict";

// Note: ES2015 Supported Browsers: https://kangax.github.io/compat-table/es6/

const DOM = {
  appWrapper: $('#app-wrapper'),
  confirmModal: $('#confirm-modal'),
  registerModal: $('#register-modal'),
  loginModal: $('#login-modal'),
  eventModal: $('#event-modal'),
};

function showError(err) {
  setTimeout(() => {
    alert(err);
  }, 100);
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
        });
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
        return fetch('/api/events/'+eventId, {
          method: 'GET',
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
      },
      reserveSheet (eventId, sheetRank) {
        return fetch('/api/events/'+eventId+'/actions/reserve', {
          method: 'POST',
          headers: new Headers({ 'Content-Type': 'application/json' }),
          body: JSON.stringify({ sheet_rank: sheetRank }),
          credentials: 'same-origin',
        }).then(handleJSON).then(handleJSONError);
      },
      freeSheet (eventId, sheetRank, sheetNum) {
        return fetch('/api/events/'+eventId+'/sheets/'+sheetRank+'/'+sheetNum+'/reservation', {
          method: 'DELETE',
          credentials: 'same-origin',
        });
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
      const message = sheetRank+': '+this.event.sheets[sheetRank].price+'円';
      confirm('Sheet Reservation', message).then(() => {
        return API.Event.reserveSheet(this.event.id, sheetRank);
      }).then(result => {
        const sheet = this.event.sheets[sheetRank].detail[result.sheet_num-1];
        sheet.reserved = true;
        sheet.mine = true;
        this.event.sheets[sheetRank].remains--;
        this.event.remains--;
        this.$forceUpdate();
      }).catch(err => {
        showError(err);
      });
    },
    freeSheet (sheetRank, sheetNum) {
      const sheet = this.event.sheets[sheetRank].detail[sheetNum-1];
      if (!sheet.mine) return;

      const message = 'Do you cancel the sheet reservation?: '+sheetRank+'-'+sheet.num;
      confirm('Cancel Sheet Reservation', message).then(() => {
        return API.Event.freeSheet(this.event.id, sheetRank, sheetNum);
      }).then(() => {
        sheet.reserved = false;
        sheet.mine = false;
        this.event.sheets[sheetRank].remains++;
        this.event.remains++;
        this.$forceUpdate();
      }).catch(err => {
        showError(err);
      });
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
  const id = setInterval(() => {
    updateEventModal(eventId).then(event => {
      EventList.$data.events.forEach((e, i, events) => {
        if (e.id !== event.id) return;
        events[i] = event;
      });
      EventList.$forceUpdate();
    });
  }, 10000);
  updateEventModal(eventId).then(() => {
    DOM.eventModal.modal('show');
    DOM.eventModal.once('hide.bs.modal', e => {
      clearInterval(id);
    });
  }).catch(err => {
    clearInterval(id);
  });
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
      confirm('Sign Out?', 'Do you really sign out?').then(() => {
        return API.User.logout();
      }).then(() => {
        this.currentUser = null;
      });
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
      API.User.login(this.loginName, this.password).then(user => {
        MenuBar.$data.currentUser = user;
        DOM.loginModal.modal('hide');
      }).catch(err => {
        showError(err);
      });;
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
      API.User.register(this.nickname, loginName, password).then(() => {
        return API.User.login(loginName, password);
      }).then(user => {
        MenuBar.$data.currentUser = user;
        DOM.registerModal.modal('hide');
      }).catch(err => {
        showError(err);
      });
    },
  },
});

$('body').on('shown.bs.modal', '.modal', e => {
  $('input', e.target).first().focus();
});
