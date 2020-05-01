function unixToLocalTime(timestamp) {
    // Timestamp is in seconds, but Date expects it in milliseconds
    return new Date(timestamp * 1000);
}

function formateUnixTimestamp(timestamp) {
    var date = unixToLocalTime(timestamp);

    return date.toLocaleTimeString();
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
  },

  computed: {
    usernameInputVisible: function() {
      return !this.joining && !this.joined;
    },
  },

  created: function() {
    var self = this;
    this.ws = new WebSocket('wss://' + window.location.host + '/ws');
    this.ws.addEventListener('close', function(e) {
      console.log('close', e)
    });
    this.ws.addEventListener('message', function(e) {
      var msg = JSON.parse(e.data);

      if (msg.type === "error") {
        if (self.joining) {
          self.joining = false
        }
        // Materialize.toast(msg.message.error, 5000);
        self.chatContent += '<div class="message-list">' +
          '<div class="msg"><span class="user">[Error]</span>' +
          msg.message.error + '</div></div>';
      } else if (msg.type === "hello") {
        if (self.joining) {
          self.joining = false;
          self.joined = true;
        }
      } else if (msg.type === "recv") {
        var time = formateUnixTimestamp(msg.message.timestamp);

        self.chatContent += '<div class="message-list">' +
          '<div class="msg">[' + time + '] ' +
          '<span class="user">' + msg.message.username + '</span>' +
          emojione.toImage(msg.message.message) // Parse emojis
          +
          '</div></div>';
      } else if (msg.type === "user_change") {
        var username = msg.message.username;
        var action = msg.message.action;
        var userCount = msg.message.user_count;
        var time = formateUnixTimestamp(msg.message.timestamp);

        var line = '';

        if (action === 'connect') {
          line = username + ' connected';
        } else if (action === 'disconnect') {
          line = username + ' disconnected';
        } else if (action === 'ban') {
          line = username + ' has been banned';
        } else {
          line = username + action + 'ed';
        }

        self.chatContent += '<div class="message-list">' +
          '<div class="msg">[' + time + '] <span class="user">[Info]</span>' +
          line + '</div></div>';

        if (userCount != null) {
            self.chatContent += '<div class="message-list">' +
              '<div class="msg"><span class="user">[Info]</span>' +
              userCount + ' user(s) in chat</div></div>';
        }
      }

      var element = document.getElementById('message-wrap');
      element.scrollTop = element.scrollHeight; // Auto scroll to the bottom
    });
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
        alert('You must choose a username');
        return
      }
      this.username = $('<p>').html(this.username).text();

      this.joining = true;

      // Send init message
      this.ws.send(JSON.stringify({
        type: "init",
        message: {
          username: this.username,
        },
      }));
    }

  }
});