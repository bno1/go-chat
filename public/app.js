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
        this.ws = new WebSocket('ws://' + window.location.host + '/ws');
        this.ws.addEventListener('message', function(e) {
            var msg = JSON.parse(e.data);

            if (msg.type === "error") {
                if (self.joining) {
                    self.joining = false
                }

                self.chatContent += '<div class="chip">Error</div>'
                    + msg.message.error + '<br/>';
            } else if (msg.type === "stats") {
                if (self.joining) {
                    self.joining = false;
                    self.joined = true;
                }

                self.chatContent += '<div class="chip">Hello</div>' +
                    msg.message.user_count + ' users in chat<br/>';
            } else if (msg.type === "recv") {
                self.chatContent += '<div class="chip">'
                    // Avatar url
                    + '<img src="' + self.gravatarURL(msg.message.username)
                    + '">' + msg.message.username
                    + '</div>'
                    + emojione.toImage(msg.message.message) // Parse emojis
                    + '<br/>';
            } else if (msg.type === "user_change") {
                var username = msg.message.username;
                var action = msg.message.action;

                var line = '';

                if (action === 'connect') {
                    line = 'User \'' + username + '\' connected';
                } else if (action === 'disconnect') {
                    line = 'User \'' + username + '\' disconnected';
                } else {
                    line = 'User \'' + username + '\' ' + action  + 'ed';
                }

                self.chatContent += '<div class="chip">User event</div>' +
                    line + '<br/>';
            }

            var element = document.getElementById('chat-messages');
            element.scrollTop = element.scrollHeight; // Auto scroll to the bottom
        });
    },

    methods: {
        send: function () {
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

        join: function () {
            if (!this.username) {
                Materialize.toast('You must choose a username', 2000);
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
        },

        gravatarURL: function(username) {
            return 'http://www.gravatar.com/avatar/' + CryptoJS.MD5(username);
        }
    }
});