// Protocol buffer type definitions for godom wire protocol.
// Matches protocol.proto — defined via protobuf.js reflection API.
var godomProto = (function() {
    var protobuf = self.protobuf; // set by protobuf.min.js (light build)

    var Root = protobuf.Root,
        Type = protobuf.Type,
        Field = protobuf.Field,
        OneOf = protobuf.OneOf;

    var root = new Root();

    // WSMessage — inner pre-built message (call/bind)
    var WSMessage = new Type("WSMessage")
        .add(new Field("type", 1, "string"))
        .add(new Field("method", 2, "string"))
        .add(new Field("args", 3, "bytes", "repeated"))
        .add(new Field("field", 4, "string"))
        .add(new Field("value", 5, "bytes"))
        .add(new Field("scope", 6, "string"));

    // Envelope — browser → Go wrapper
    var Envelope = new Type("Envelope")
        .add(new Field("args", 1, "double", "repeated"))
        .add(new Field("msg", 2, "bytes"))
        .add(new Field("value", 3, "bytes"));

    // EventCommand — event listener config
    var EventCommand = new Type("EventCommand")
        .add(new Field("id", 1, "string"))
        .add(new Field("on", 2, "string"))
        .add(new Field("key", 3, "string"))
        .add(new Field("msg", 4, "bytes"));

    // Command — single DOM operation
    var Command = new Type("Command")
        .add(new Field("op", 1, "string"))
        .add(new Field("id", 2, "string"))
        .add(new Field("name", 3, "string"))
        .add(new Field("strVal", 4, "string"))
        .add(new Field("boolVal", 5, "bool"))
        .add(new Field("numVal", 6, "double"))
        .add(new Field("rawVal", 7, "bytes"))
        .add(new OneOf("val", ["strVal", "boolVal", "numVal", "rawVal"]))
        .add(new Field("items", 8, "ListItem", "repeated"));

    // ListItem — single g-for list item
    var ListItem = new Type("ListItem")
        .add(new Field("html", 1, "string"))
        .add(new Field("cmds", 2, "Command", "repeated"))
        .add(new Field("evts", 3, "EventCommand", "repeated"));

    // ServerMessage — top-level Go → browser message
    var ServerMessage = new Type("ServerMessage")
        .add(new Field("type", 1, "string"))
        .add(new Field("commands", 2, "Command", "repeated"))
        .add(new Field("events", 3, "EventCommand", "repeated"));

    root.add(ServerMessage);
    root.add(Command);
    root.add(ListItem);
    root.add(EventCommand);
    root.add(Envelope);
    root.add(WSMessage);

    return {
        ServerMessage: ServerMessage,
        Command: Command,
        ListItem: ListItem,
        EventCommand: EventCommand,
        Envelope: Envelope,
        WSMessage: WSMessage
    };
})();
