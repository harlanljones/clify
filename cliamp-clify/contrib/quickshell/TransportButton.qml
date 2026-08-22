// Borderless transport button with hover color shift, drawing a MediaIcon
// instead of a font glyph. shape: "prev" | "play" | "pause" | "next" | "stop".

import QtQuick

Item {
    id: root
    property string shape: "play"
    property color fgColor:    "#d4be98"
    property color hoverColor: "#d8a657"
    property bool enabled: true
    property real iconSize: 14
	property string accessibleName: shape
    signal activated()

    property bool hovered: false

    implicitWidth: iconSize + 12
    implicitHeight: iconSize + 8
	activeFocusOnTab: enabled
	Accessible.role: Accessible.Button
	Accessible.name: accessibleName
	Accessible.onPressAction: if (root.enabled) root.activated()
	Keys.onSpacePressed: if (root.enabled) root.activated()
	Keys.onReturnPressed: if (root.enabled) root.activated()

    MediaIcon {
        anchors.centerIn: parent
        shape: root.shape
        size: root.iconSize
        color: root.hovered && root.enabled ? root.hoverColor : root.fgColor
        opacity: root.enabled ? 1.0 : 0.35
    }

    MouseArea {
        anchors.fill: parent
        hoverEnabled: true
        cursorShape: root.enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
        onEntered: root.hovered = true
        onExited:  root.hovered = false
        onClicked: { if (root.enabled) root.activated() }
    }
}
