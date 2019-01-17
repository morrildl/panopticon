// Copyright © 2018 Playground Global, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
 * Util Functions
 */
 
// helper function, because lolJavaScript
function str(s) {
  if ((s !== undefined) && (s !== null) && (s !== "")) {
    return s;
  }
  return "";
}

const globals = {
  Cameras: [],
  ServiceName: "Panopticon",
  DefaultPath: "",
  CurrentCameraName: "No camera",
};

const generalError = { Message: "An error occurred in this app.", Extra: "Please reload this page.", Recoverable: false };

Vue.component('waiting-modal', {
  template: "#waiting-modal",
  props: [ "message", "waiting" ],
  computed: {
    displayMessage: function() {
      return str(this.message) != "" ? str(this.message) : "A moment please...";
    },
  },
});

Vue.component('error-modal', {
  template: "#error-modal",
  props: [ "error", "clear" ],
  computed: {
    visible: function() {
      return str(this.error.message) != "";
    },
  },
});

const waitingMixin = {
  data: function() {
    return {
      waiting: false,
    };
  },
  methods: {
    wait: function() {
      this.waiting = true;
    },
    clearWait: function() {
      this.waiting = false;
    },
    callAPI: function(url, method, payload={}, onArtifact=()=>{}, onError=()=>{}, onFinal=()=>{}) {
      this.wait();
      axios({
        url: url,
        method: method,
        data: payload,
      }).then((res) => {
        if (res && res.data && res.data.Artifact) {
          onArtifact(res.data.Artifact);
        } else {
          let error = res.data.Error ? res.data.Error : generalError;
          onError(res.status, error);
        }
        this.clearWait();
        onFinal();
      }).catch((err) => {
        if (err.response) {
          let data = err.response.data && err.response.data.Error ? err.response.data.Error : null;
          onError(err.response.status, data);
        }
        this.clearWait();
        onFinal();
      });
    }
  },
};

const errorMixin = {
  data: function() {
    return {
      error: {
        message: "",
        extra: "",
        recoverable: false,
      },
    };
  },
  methods: {
    setError: function(code, error) {
      if (!error) return;
      this.error = {
        message: error.Message || "",
        extra: error.Extra || "",
        recoverable: error.IsRecoverable ? true : false,
      };
    },
    clearError: function() {
      if (!this.error.recoverable) return;
      this.error = {
        message: "",
        extra: "",
        recoverable: false,
      };
    },
  },
};

const confirmMixin = {
  data: function() {
    return {
      challenge: null,
    };
  },
  methods: {
    confirm: function(title, message, proceedText, abortText, proceed) {
      this.challenge = {
        title: title, 
        body: message.split("\n"),
        proceedText: proceedText,
        abortText: abortText,
        proceed: () => { proceed(); this.challenge = null; },
        abort: () => { this.challenge = null; },
      };
    },
  },
};

const settings = Vue.component('settings', {
  template: "#settings",
  props: [ "globals" ],
  mounted: function() {
    axios.get("/api/config").then((res) => {
      if (res.data.Artifact) {
        this.serviceName = res.data.Artifact.ServiceName;
        this.clientLimit = res.data.Artifact.ClientLimit;
        this.clientCertDuration = res.data.Artifact.IssuedCertDuration;
        this.whitelistedDomains = res.data.Artifact.WhitelistedDomains;
      } else {
        this.error = res.data.Error ? res.data.Error : generalError;
      }
    }).catch((err) => {
      this.error = err.response.data.Error ? err.response.data.Error : generalError;
    });
  },
  data: function() {
    return {
      serviceName: "",
      clientLimit: "",
      clientCertDuration: "",
      whitelistedDomains: "",
      xhrPending: false,
      error: { },
    };
  },
  methods: {
    clearError: function() { this.error = { }; },
    cancel: function() {
      this.$router.push(globals.DefaultPath);
    },
    submit: function() {
      let whitelistedDomains = str(""+this.whitelistedDomains).split(" ").filter(w => w != "");
      let payload = {
        ServiceName: this.serviceName,
        ClientLimit: parseInt(this.clientLimit),
        IssuedCertDuration: parseInt(this.clientCertDuration),
        WhitelistedDomains: whitelistedDomains,
      };
      if (payload.ClientLimit == NaN) {
        this.error = {Message: "Max clients must be a number.", Extra: "", Recoverable: true};
        return;
      }
      if (payload.IssuedCertDuration == NaN) {
        this.error = {Message: "Refresh period must be a number.", Extra: "", Recoverable: true};
        return;
      }
      axios.put("/api/config", json=payload).then((res) => {
        this.$router.push(globals.DefaultPath);
        document.location.reload();
      }).catch((err) => {
        this.error = err.response.data.Error ? err.response.data.Error : generalError;
      });
    },
  },
});

const events = Vue.component('state-fetcher', {
  template: "#state-fetcher",
  mixins: [waitingMixin, errorMixin],
  data: function() {
    return {
      refreshTimer: null,
      globals: globals,
    };
  },
  methods: {
    loadState: function() {
      this.callAPI("/client/state", "get", null, (artifact) => {
        globals.Cameras = artifact.Cameras;
        globals.ServiceName = str(artifact.ServiceName);
        document.title = globals.ServiceName;

        if (str(this.$route.path) == "/" || str(this.$route.path) == "") {
          if (globals.Cameras.length > 0) {
            this.$router.replace(`/camera/${globals.Cameras[0].ID}`);
          } else {
            this.$router.replace("/nocameras")
          }
        }

        globals.CurrentCameraName = "No camera";
        for (let c of globals.Cameras) {
          if (c.ID == this.$route.params.camera) {
            globals.CurrentCameraName = c.Name;
          }
        }
      }, this.setError);
    },
    startRefresh: function() {
      this.loadState();
      this.refreshTimer = setInterval(() => { this.loadState(); }, 5000);
    },
    stopRefresh: function() {
      if (this.refreshTimer != null) {
        clearInterval(this.refreshTimer);
        this.refreshTimer = null;
      }
    },
  },
  watch: {
    '$route': function(to, from) {
      // force an immediate state refresh whenever user changes selected camera
      this.stopRefresh();
      this.startRefresh();
    }
  },
  mounted: function() {
    this.startRefresh();
  },
  beforeDestroy: function() {
    this.stopRefresh();
  },
});

Vue.component('navbar', { 
  template: "#navbar", 
  mixins: [waitingMixin, errorMixin],
  data: function() { return { globals: globals }},
  methods: {
    changeCamera: function(cID) {
      this.$router.push(`/camera/${cID}`);
    }
  }
});

const noCameras = Vue.component('noCameras', {
  template: "#no-cameras",
  props: ["globals"],
});

const camera = Vue.component('camera', {
  template: "#camera",
  props: ["globals"],
});

const router = new VueRouter({
  mode: "history",
  base:  "/",
  routes: [
    { path: "/nocameras", component: noCameras, props: {globals: globals} },
    { path: "/camera/:camera", component: camera, props: {globals: globals} },
    { path: "/settings", component: settings, props: {globals: globals} },

    //{ path: "/users/:email", component: userDetails, props: (route) => ({ globals: globals, email: route.params.email })},
  ],
});

new Vue({el: "#panopticon-root", router: router});