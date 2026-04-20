// Protocol buffer type definitions for godom wire protocol.
// IMPORTANT: This file must stay in sync with protocol.proto.
// Update this file whenever protocol.proto changes.
var godomProto = (function() {
    var protobuf = self.protobuf; // set by protobuf.min.js (light build)

    var Root = protobuf.Root,
        Type = protobuf.Type,
        Field = protobuf.Field,
        Enum = protobuf.Enum;

    var root = new Root();

    // --- Enums ---

    var ServerKind = new Enum("ServerKind", {
        SERVER_INIT: 0,
        SERVER_PATCH: 1,
        SERVER_JSCALL: 2,
        SERVER_STREAM: 3,
        SERVER_BROADCAST: 4
    });

    var BrowserKind = new Enum("BrowserKind", {
        BROWSER_INPUT: 0,
        BROWSER_METHOD: 1,
        BROWSER_JSRESULT: 2,
        BROWSER_INIT_REQUEST: 3,
        BROWSER_PAGE_INFO: 4,
        BROWSER_BROADCAST: 5
    });

    // --- DomPatch ---

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

    // --- ServerMessage ---

    var ServerMessage = new Type("ServerMessage")
        .add(ServerKind)
        .add(new Field("kind", 1, "ServerKind"))
        .add(new Field("target", 2, "string"))
        .add(new Field("tree", 10, "bytes"))
        .add(new Field("patches", 11, "DomPatch", "repeated"))
        .add(new Field("callId", 20, "int32"))
        .add(new Field("expr", 21, "string"));

    // --- BrowserMessage ---

    var BrowserMessage = new Type("BrowserMessage")
        .add(BrowserKind)
        .add(new Field("kind", 1, "BrowserKind"))
        .add(new Field("nodeId", 2, "int32"))
        .add(new Field("value", 10, "string"))
        .add(new Field("method", 20, "string"))
        .add(new Field("args", 21, "bytes", "repeated"))
        .add(new Field("callId", 30, "int32"))
        .add(new Field("result", 31, "bytes"))
        .add(new Field("error", 32, "string"))
        .add(new Field("island", 40, "string"));

    root.add(ServerMessage);
    root.add(DomPatch);
    root.add(BrowserMessage);

    // Enum constants for use in bridge.js
    var SK = ServerKind.values;
    var BK = BrowserKind.values;

    return {
        ServerMessage: ServerMessage,
        DomPatch: DomPatch,
        BrowserMessage: BrowserMessage,
        ServerKind: SK,
        BrowserKind: BK
    };
})();
