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

            if (msg.error !== undefined) {
                if (self.joining) {
                    self.joining = false
                }

                self.chatContent += '<div class="chip">Error</div>'
                    + msg.error + '<br/>';
            } else if (msg.status !== undefined) {
                if (self.joining) {
                    self.joining = false;
                    self.joined = true;
                }

                self.chatContent += '<div class="chip">Status</div>'
                    + msg.status + ', Users: ' + msg.user_count + '<br/>';
            } else {
                self.chatContent += '<div class="chip">'
                        + '<img src="' + self.gravatarURL(msg.username) + '">' // Avatar
                        + msg.username
                    + '</div>'
                    + emojione.toImage(msg.message) + '<br/>'; // Parse emojis
            }

            var element = document.getElementById('chat-messages');
            element.scrollTop = element.scrollHeight; // Auto scroll to the bottom
        });
    },

    methods: {
        send: function () {
            if (this.newMsg != '') {
                this.ws.send(JSON.stringify({
                    message: $('<p>').html(this.newMsg).text() // Strip out html
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
                username: this.username
            }));
        },

        gravatarURL: function(username) {
            return 'http://www.gravatar.com/avatar/' + CryptoJS.MD5(username);
        }
    }
});