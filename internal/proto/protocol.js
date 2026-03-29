// Protocol buffer type definitions for godom VDOM wire protocol.
// IMPORTANT: This file must stay in sync with protocol.proto.
// Update this file whenever protocol.proto changes.
var godomProto = (function() {
    var protobuf = self.protobuf; // set by protobuf.min.js (light build)

    var Root = protobuf.Root,
        Type = protobuf.Type,
        Field = protobuf.Field;

    var root = new Root();

    // NodeEvent — browser → Go, Layer 1: just node ID + value (tag byte 0x01)
    var NodeEvent = new Type("NodeEvent")
        .add(new Field("nodeId", 1, "int32"))
        .add(new Field("value", 2, "string"));

    // MethodCall — browser → Go, Layer 2: method dispatch (tag byte 0x02)
    var MethodCall = new Type("MethodCall")
        .add(new Field("nodeId", 1, "int32"))
        .add(new Field("method", 2, "string"))
        .add(new Field("args", 3, "bytes", "repeated"));

    // DomPatch — single DOM mutation from diff
    var DomPatch = new Type("DomPatch")
        .add(new Field("nodeId", 1, "int32"))
        .add(new Field("op", 2, "string"))
        .add(new Field("text", 10, "string"))
        .add(new Field("facts", 11, "bytes"))
        .add(new Field("treeContent", 12, "bytes"))
        .add(new Field("count", 13, "int32"))
        .add(new Field("reorder", 14, "bytes"))
        .add(new Field("pluginData", 15, "bytes"))
        .add(new Field("subPatches", 16, "DomPatch", "repeated"));

    // VDomMessage — top-level Go → browser message
    var VDomMessage = new Type("VDomMessage")
        .add(new Field("type", 1, "string"))
        .add(new Field("patches", 3, "DomPatch", "repeated"))
        .add(new Field("tree", 5, "bytes"))
        .add(new Field("targetName", 6, "string"));

    root.add(VDomMessage);
    root.add(DomPatch);
    root.add(NodeEvent);
    root.add(MethodCall);

    return {
        VDomMessage: VDomMessage,
        DomPatch: DomPatch,
        NodeEvent: NodeEvent,
        MethodCall: MethodCall
    };
})();
