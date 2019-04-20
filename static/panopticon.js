// Copyright © 2019 Dan Morrill
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

Vue.use(Buefy.default);
Vue.use(VueViewer.default);

const globals = new Vuex.Store({
  state: {
    ServiceName: "Panopticon",
    DefaultPath: "",
    DefaultImage: "/static/no-image.png",
    Cameras: [],
    CurrentCamera: {
      Name: "No camera",
      LatestHandle: "",
    },
  },
  mutations: {
    "cameras": function(state, cameras) {
      state.Cameras = cameras;
    },
    "service-name": function(state, name) {
      state.ServiceName = name;
    },
    "default-image": function(state, uri) {
      state.DefaultImage = uri;
    },
    "default-path": function(state, path) {
      state.DefaultPath = path;
    },
    "current-camera": function(state, cam) {
      state.CurrentCamera = cam;
    },
  },
});

const generalError = { Message: "An error occurred in this app.", Extra: "Please reload this page.", Recoverable: false };

Vue.component('error-modal', {
  template: "#error-modal",
  props: [ "error", "clear" ],
  computed: {
    visible: function() {
      return this.$str(this.error.message) != "";
    },
  },
});

const apiMixin = {
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
      this.$router.push(this.$store.state.DefaultPath);
    },
    submit: function() {
      let whitelistedDomains = this.$str(""+this.whitelistedDomains).split(" ").filter(w => w != "");
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
        this.$router.push(this.$store.state.DefaultPath);
        document.location.reload();
      }).catch((err) => {
        this.error = err.response.data.Error ? err.response.data.Error : generalError;
      });
    },
  },
});

const events = Vue.component('state-fetcher', {
  template: "#state-fetcher",
  mixins: [apiMixin, errorMixin],
  data: function() {
    return {
      refreshTimer: null,
    };
  },
  methods: {
    loadState: function() {
      this.callAPI("/client/state", "get", null, (artifact) => {
        this.$store.commit("cameras", artifact.Cameras);
        this.$store.commit("service-name", this.$str(artifact.ServiceName));
        this.$store.commit("default-image", this.$str(artifact.DefaultImage));
        this.$store.commit("default-path", this.$str(artifact.DefaultPath));
        document.title = this.$store.state.ServiceName;

        if (this.$str(this.$route.path) == "/" || this.$str(this.$route.path) == "") {
          if (this.$store.state.Cameras.length > 0) {
            this.$router.replace(`/camera/${this.$store.state.Cameras[0].ID}`);
          } else {
            this.$router.replace("/nocameras")
          }
        }

        this.$store.commit("current-camera", { Name: "No camera", LatestHandle: "" });
        for (let c of this.$store.state.Cameras) {
          if (c.ID == this.$route.params.camera) {
            this.$store.commit("current-camera", c);
            break;
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

const noCameras = Vue.component('noCameras', {
  template: "#no-cameras",
});

const saveMixin = {
  methods: {
    save: function(handle) {
      if (this.$str(handle) == "") {
        return;
      }
      this.callAPI(`/client/save/${handle}`, "put", null, (artifact) => {
      }, this.setError);
    },
    imgURI: function(handle) {
      if (this.$str(handle) == "") {
        return this.$store.state.DefaultImage;
      }
      return `/client/image/${handle}`;
    },
    currentImg: function() {
      return this.imgURI(this.$store.state.CurrentCamera.LatestHandle);
    },
  }
};

const camera = Vue.component('camera', {
  template: "#camera",
  mixins: [apiMixin, errorMixin, saveMixin],
  methods: {
    changeCamera: function(cID) {
      this.$router.push(`/camera/${cID}`);
    },
    fetchImg: function(typ, slot) {
      if (this.$store.state.CurrentCamera == undefined || this.$store.state.CurrentCamera[typ] == undefined) {
        return undefined;
      }
      return this.$store.state.CurrentCamera[typ][slot];
    },
    settings: function() {
      this.$router.push("/settings");
    },
  }
});

const playerMixin = {
  data: function() {
    return {
      showVidya: false,
      vsrc: '',
    };
  },
  methods: {
    display: function(event) {
      if (this.img != undefined && this.img.HasVideo) {
        event.stopPropagation();
        this.showVidya = true;
        this.vsrc = `/client/video/${this.img.Handle}`;
      }
    },
  },
  computed: {
    src: function() {
      if (this.$str(this.img) == "") {
        return this.$store.state.DefaultImage;
      }
      return `/client/image/${this.img.Handle}`;
    },
  },
};

// a thumbnail is a smaller, unadorned instance of an image, intended as, well, a thumbnail
const thumbnail = Vue.component('thumbnail', {
  template: "#thumbnail",
  mixins: [playerMixin],
  props: ["img"],
});

// a gallery item is a slightly larger instance of an image with a Save button, for use in a lightbox UI
const galleryItem = Vue.component('gallery-item', {
  template: "#gallery-item",
  mixins: [playerMixin],
  props: {
    "img": String,
    "onsave": Object,
    "caption": String,
    "nosave": { type: Boolean, default: false },
  },
});

const gallery = Vue.component('gallery', {
  template: "#gallery",
  mixins: [apiMixin, errorMixin, saveMixin],
  data: function() {
    return {
      results: 0,
      camera: "",
      imageList: [],
      current: 1,
      per: 9,
    };
  },
  computed: {
    kind: function() {
      return { 
        "collected": "Recent images",
        "generated": "Timelapses",
        "saved": "Saved items",
        "motion": "Motion-captured images"
      }[this.$route.params.kind];
    },
    skip: function() {
      return (this.current - 1) * this.per;
    },
  },
  mounted: function() {
    this.update();
  },
  methods: {
    camView: function() {
      this.$router.push(`/camera/${this.$route.params.camera}`)
    },
    page: function() {
      this.callAPI(`/client/images/${this.$route.params.camera}/${this.$route.params.kind}?skip=${this.skip}&per=${this.per}`, "get", null, (artifact) => {
        this.imageList = artifact.Images ? artifact.Images : [];
        this.camera = artifact.Camera;
        this.results = artifact.Total;
      });
    },
    update: function(value) {
      if (value) { this.current = value; }
      this.page();
    },
  },
});

const router = new VueRouter({
  mode: "history",
  base:  "/",
  routes: [
    { path: "/nocameras", component: noCameras },
    { path: "/camera/:camera", component: camera },
    { path: "/gallery/:camera/:kind", component: gallery },
    { path: "/settings", component: settings },
  ],
});

Vue.prototype.$vvdefaults = {
  inline: false, button:false, title:false, rotatable:false, scalable:false, navbar: false, toolbar: false,
};

// helper function, because lolJavaScript
Vue.prototype.$str = function(s) {
  if ((s !== undefined) && (s !== null) && (s !== "")) {
    return s;
  }
  return "";
}

new Vue({el: "#panopticon-root", store: globals, router: router});