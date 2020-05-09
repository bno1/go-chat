function unixToLocalTime(timestamp) {
    // Timestamp is in seconds, but Date expects it in milliseconds
    return new Date(timestamp * 1000);
    
}

function formateUnixTimestamp(timestamp) {
    var date = unixToLocalTime(timestamp);

    return date.getHours() + ':' + (date.getMinutes() < 10? '0' : '') + date.getMinutes();
}

new Vue({
  el: '#app',

  data: {
    ws: null, // Our websocket
    newMsg: '', // Holds new messages to be sent to the server
    chatContent: '', // A running list of chat messages displayed on the screen
    username: null, // Our username
    joining: false, // True if username has been fielled in
    joined: false, // True if username has been accepted
    notice: '',
    usersOnline: 0,
    settingsOpen: false,
  },

  computed: {
    usernameInputVisible: function() {
      return !this.joining && !this.joined;
    },
  },

  mounted: function() {
    this.username = this.getUsername();
  },

  created: function() {
    var self = this;

    this.connectToChat();

    this.ws.addEventListener('close', function(e) {
      self.joining = false;
      self.joined = false;

      self.showNotice("Ai fost deconectat de la server.");
      self.ws.isAlive = false;
      self.ws = null;

      self.connectToChat();
      self.processMessages();

    });

    this.ws.addEventListener('error', function(e) {
      console.log('eroare: ',e);
    });

    this.processMessages();
  },

  methods: {
    send: function() {
      if (this.newMsg != '') {
        this.ws.send(JSON.stringify({
          type: "send",
          message: {
            // Strip out html
            message: $('<p>').html(this.newMsg).text(),
          },
        }));
        this.newMsg = ''; // Reset newMsg
      }
    },

    join: function() {
      if (!this.username) {
        this.showNotice('Trebuie sa-ti alegi un username pentru a te conecta.', 5);
        return
      }

      this.notice = ''; // reset notice

      this.setCookie('user', this.username);

      this.username = $('<p>').html(this.username).text();

      this.joining = true;

      // Send init message
      this.ws.send(JSON.stringify({
        type: "init",
        message: {
          username: this.username,
        },
      }));
    },

    connectToChat: function() {
      this.ws = new WebSocket('wss://' + window.location.host + '/ws');
      // console.log('connecting',this.ws, typeof(this.ws));
    },

    processMessages: function(){
      var self = this;
      this.ws.addEventListener('message', function(e) {
        var msg = JSON.parse(e.data);

        if (msg.type === "error") {
          if (self.joining) {
            self.joining = false
          }
          self.showNotice(msg.message.error, 5);

          self.chatScrollToBottom();
        }

        else if (msg.type === "hello") {
          if (self.joining) {
            self.joining = false;
            self.joined = true;
          }
        } 

        else if (msg.type === "stats") {
          if (self.joining) {
            self.joining = false;
            self.joined = true;
          }
          self.setOnlineUsersCount(msg.message.user_count);

          self.chatScrollToBottom();
        }

        else if (msg.type === "recv") {
          var time = formateUnixTimestamp(msg.message.timestamp);
          var cls = (msg.message.username == self.username ? 'message-list me' : 'message-list')
          self.chatContent += '<div class="'+ cls +'">' +
            '<div class="user">' + msg.message.username + '</div>' +
            '<div class="msg">' +
            emojione.toImage(msg.message.message) + // Parse emojis
            '</div>'+
            '<div class="time">'+ time +'</div>' +
            '</div>';

            self.chatScrollToBottom();
        }

        else if (msg.type === "user_change") {
          var username = msg.message.username;
          var action = msg.message.action;
          var user_list = msg.message.user_list;

          var time = formateUnixTimestamp(msg.message.timestamp);
          self.setOnlineUsersCount(msg.message.user_count);

          var line = '';

          if (username == self.username) {
            if (action === 'connect') {
              line = username + ', te-ai conectat.';
            } else if (action === 'disconnect') {
              line = username + ', ai fost deconectat.';
            } else if (action === 'ban') {
              line = username + ', ai fost banat. :(';
            } else {
              line = username + ' ' + action + 'ed';
            }
          }
          else {
            if (action === 'connect') {
              line = username + ' s-a conectat.';
            } else if (action === 'disconnect') {
              line = username + ' s-a deconectat.';
            } else if (action === 'ban') {
              line = username + ' a fost banat. :(';
            } else {
              line = username + action + 'ed';
            }
          }

          self.chatContent += '<div class="message-list">' +
            '<div class="system">[Info]</div>' +
            '<div class="msg">' +
            line +
            '</div>' +
            '<div class="time">'+ time +'</div>' +
            '</div>';

          self.chatContent += '<div class="message-list">' +
            '<div class="system">[Info]</div>' +
            '<div class="msg">User in chat: ' +
            user_list.join(', ') +
            '</div>' +
            '<div class="time">'+ time +'</div>' +
            '</div>';

          self.chatScrollToBottom();
        }

        self.chatScrollToBottom();

      });
    },

    chatScrollToBottom: function() {
      setTimeout(function(){
        var element = document.getElementById('message-wrap');
        element.scrollTop = element.scrollHeight + 10; // Auto scroll to the bottom
      }, 200);
    },

    openSettings: function() {

    },

    setOnlineUsersCount: function(nr) {
      this.usersOnline = nr;
    },

    showNotice: function(message, seconds) {
      this.notice = message;
      var self = this;

      if (seconds !== undefined) {
        setTimeout(function(){
          self.notice = '';
        }, seconds * 1000);
      }
    },

    getUsername: function() {
      var user = this.getCookie('user');

      if (user !== null) {
        return user;
      }
      return null;
    },

    // cookie utils

    getCookie: function(name) {
      var v = document.cookie.match('(^|;) ?' + name + '=([^;]*)(;|$)');
      return v ? v[2] : null;
    },

    setCookie: function (name, value, days) {
      var d = new Date;
      d.setTime(d.getTime() + 24*60*60*1000*days);
      document.cookie = name + "=" + value + ";path=/;expires=" + d.toGMTString();
    },

    deleteCookie: function(name) { setCookie(name, '', -1); }

  }
});