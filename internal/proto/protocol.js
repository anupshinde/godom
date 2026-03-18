// Protocol buffer type definitions for godom VDOM wire protocol.
// Matches the VDomMessage/DomPatch/EventSetup messages in protocol.proto.
var godomProto = (function() {
    var protobuf = self.protobuf; // set by protobuf.min.js (light build)

    var Root = protobuf.Root,
        Type = protobuf.Type,
        Field = protobuf.Field;

    var root = new Root();

    // WSMessage — inner pre-built message (call/bind), unchanged
    var WSMessage = new Type("WSMessage")
        .add(new Field("type", 1, "string"))
        .add(new Field("method", 2, "string"))
        .add(new Field("args", 3, "bytes", "repeated"))
        .add(new Field("field", 4, "string"))
        .add(new Field("value", 5, "bytes"))
        .add(new Field("scope", 6, "string"));

    // Envelope — browser → Go wrapper, unchanged
    var Envelope = new Type("Envelope")
        .add(new Field("args", 1, "double", "repeated"))
        .add(new Field("msg", 2, "bytes"))
        .add(new Field("value", 3, "bytes"));

    // EventSetup — event listener registration
    var EventSetup = new Type("EventSetup")
        .add(new Field("gid", 1, "string"))
        .add(new Field("event", 2, "string"))
        .add(new Field("key", 3, "string"))
        .add(new Field("msg", 4, "bytes"))
        .add(new Field("stopPropagation", 5, "bool"))
        .add(new Field("preventDefault", 6, "bool"));

    // DomPatch — single DOM mutation from diff
    var DomPatch = new Type("DomPatch")
        .add(new Field("index", 1, "int32"))
        .add(new Field("op", 2, "string"))
        .add(new Field("text", 10, "string"))
        .add(new Field("facts", 11, "bytes"))
        .add(new Field("htmlContent", 12, "bytes"))
        .add(new Field("count", 13, "int32"))
        .add(new Field("reorder", 14, "bytes"))
        .add(new Field("pluginData", 15, "bytes"))
        .add(new Field("subPatches", 16, "DomPatch", "repeated"))
        .add(new Field("patchEvents", 17, "EventSetup", "repeated"));

    // VDomMessage — top-level Go → browser message
    var VDomMessage = new Type("VDomMessage")
        .add(new Field("type", 1, "string"))
        .add(new Field("html", 2, "bytes"))
        .add(new Field("patches", 3, "DomPatch", "repeated"))
        .add(new Field("events", 4, "EventSetup", "repeated"));

    root.add(VDomMessage);
    root.add(DomPatch);
    root.add(EventSetup);
    root.add(Envelope);
    root.add(WSMessage);

    return {
        VDomMessage: VDomMessage,
        DomPatch: DomPatch,
        EventSetup: EventSetup,
        Envelope: Envelope,
        WSMessage: WSMessage
    };
})();
