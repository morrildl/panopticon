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
  DefaultImage: "/static/no-image.png",
  CurrentCamera: {
    Name: "No camera",
    LatestHandle: "",
  },
};

const generalError = { Message: "An error occurred in this app.", Extra: "Please reload this page.", Recoverable: false };

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
        globals.DefaultImage = str(artifact.DefaultImage);
        document.title = globals.ServiceName;

        if (str(this.$route.path) == "/" || str(this.$route.path) == "") {
          if (globals.Cameras.length > 0) {
            this.$router.replace(`/camera/${globals.Cameras[0].ID}`);
          } else {
            this.$router.replace("/nocameras")
          }
        }

        globals.CurrentCamera = { Name: "No camera", LatestHandle: "" };
        for (let c of globals.Cameras) {
          if (c.ID == this.$route.params.camera) {
            globals.CurrentCamera = c;
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
  props: ["globals"],
});

const pinMixin = {
  methods: {
    pin: function(handle) {
      if (str(handle) == "") {
        return;
      }
      this.callAPI(`/client/pin/${handle}`, "put", null, (artifact) => {
      }, this.setError);
    },
    imgURI: function(handle) {
      if (str(handle) == "") {
        return "/static/no-image.png";
      }
      return `/client/image/${handle}`;
    },
    currentImg: function() {
      return this.imgURI(globals.CurrentCamera.LatestHandle);
    },
  }
};

const thumbnail = Vue.component('thumbnail', {
  template: "#thumbnail",
  props: ["img", "isvideo", "vvdefaults"],
  computed: {
    src: function() {
      if (str(this.img) == "") {
        return "/static/no-image.png";
      }
      return `/client/image/${this.img.Handle}`;
    },
  },
  methods: {
    display: function() {
      if (this.img != undefined && this.img.HasVideo) {
        this.$router.push(`/player/${this.img.Handle}`);
      }
    },
  },
});

const camera = Vue.component('camera', {
  template: "#camera",
  mixins: [waitingMixin, errorMixin, pinMixin],
  props: ["globals", "vvdefaults"],
  data: function() {
    return {
      lightboxUp: false,
    }
  },
  methods: {
    changeCamera: function(cID) {
      this.$router.push(`/camera/${cID}`);
    },
    savedImg: function(slot) {
      return this.slotImg("Pinned", slot).Handle;
    },
    motionImg: function(slot) {
      return this.slotImg("Motion", slot).Handle;
    },
    timelapseImg: function(slot) {
      return this.slotImg("Timelapse", slot).Handle;
    },
    play: function(slot) {
      if (this.globals.CurrentCamera == undefined || this.globals.CurrentCamera["Timelapse"] == undefined) {
        return;
      }
      if (this.globals.CurrentCamera["Timelapse"][slot] != undefined) {
        let handle = this.globals.CurrentCamera["Timelapse"][slot].Handle;
        this.$router.push(`/player/${handle}`);
      } else {
        return;
      }
    },
    recentImg: function(slot) {
      return this.slotImg("Recent", slot).Handle;
    },
    fetchImg: function(typ, slot) {
      if (this.globals.CurrentCamera == undefined || this.globals.CurrentCamera[typ] == undefined) {
        return undefined;
      }
      return this.globals.CurrentCamera[typ][slot];
    },
    slotImg: function(typ, slot) {
      if (this.globals.CurrentCamera == undefined || this.globals.CurrentCamera[typ] == undefined) {
        return "/static/no-image.png";
      }
      if (this.globals.CurrentCamera[typ][slot] != undefined) {
        return this.imgURI(this.globals.CurrentCamera[typ][slot]);
      } else {
        return "/static/no-image.png";
      }
    },
    settings: function() {
      this.$router.push("/settings");
    },
  }
});

const lightbox = Vue.component('lightbox', {
  template: "#lightbox",
  mixins: [waitingMixin, errorMixin, pinMixin],
  props: ["globals"],
  data: function() {
    return {
      date: "",
      time: "",
      camera: "",
      isPinned: false,
    };
  },
  mounted: function() {
    this.callAPI(`/client/imagemeta/${this.$route.params.image}`, "get", null, (artifact) => {
      this.date = artifact.Date;
      this.time = artifact.Time;
      this.camera = artifact.Camera;
      this.isPinned = artifact.IsPinned ? true : false;
    });
  },
  methods: {
  },
});

const galleryItem = Vue.component('gallery-item', {
  template: "#gallery-item",
  props: ["img", "onsave", "caption", "vvdefaults"],
  data: function() {
    return { };
  },
  methods: {
    display: function() {
      if (this.img != undefined && this.img.HasVideo) {
        this.$router.push(`/player/${this.img.Handle}`);
      }
    },
  },
});

const gallery = Vue.component('gallery', {
  template: "#gallery",
  mixins: [waitingMixin, errorMixin, pinMixin],
  props: ["globals", "vvdefaults"],
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
        "collected": "recent images",
        "generated": "timelapses",
        "pinned": "saved images",
        "motion": "motion-captured images"
      }[this.$route.params.kind];
    },
    isPlayable: function() {
      return this.$route.params.kind == "generated";
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
    play: function(img) {
      if (img == undefined || !img.HasVideo) {
        return;
      }
      this.$router.push(`/player/${img.Handle}`);
    },
  },
});

const vidya = Vue.component('vidya', {
  template: "#vidya",
  mixins: [waitingMixin, errorMixin],
  props: ["globals"],
  data: function() {
    return {
    };
  },
});

const vvdefaults = {
  inline: false, button:false, title:false, rotatable:false, scalable:false, navbar: false, toolbar: false,
};

const router = new VueRouter({
  mode: "history",
  base:  "/",
  routes: [
    { path: "/nocameras", component: noCameras, props: {globals: globals} },
    { path: "/camera/:camera", component: camera, props: {globals: globals, vvdefaults: vvdefaults} },
    { path: "/image/:image", component: lightbox, props: {globals: globals} },
    { path: "/gallery/:camera/:kind", component: gallery, props: {globals: globals, vvdefaults: vvdefaults} },
    { path: "/settings", component: settings, props: {globals: globals} },
    { path: "/player/:handle", component: vidya, props: {globals: globals} },

    //{ path: "/users/:email", component: userDetails, props: (route) => ({ globals: globals, email: route.params.email })},
  ],
});

new Vue({el: "#panopticon-root", router: router});