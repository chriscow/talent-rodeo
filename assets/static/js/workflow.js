let editor;
let ws;

document.addEventListener('DOMContentLoaded', function () {
    editor = new Drawflow(document.getElementById('drawflow'));
    editor.start();

    // Register node types
    editor.registerNode('sequential', sequentialTemplate);
    editor.registerNode('must', mustTemplate);
    editor.registerNode('first', firstTemplate);
    editor.registerNode('for', forTemplate);
    editor.registerNode('range', rangeTemplate);
    editor.registerNode('policy', policyTemplate);

    connectWebSocket();
});

function connectWebSocket() {
    ws = new WebSocket('ws://' + window.location.host + '/ws');
    ws.onmessage = handleWSMessage;
    ws.onerror = () => console.error('WebSocket error');
    ws.onclose = () => setTimeout(connectWebSocket, 1000); // Reconnect on close
}

function handleWSMessage(event) {
    const msg = JSON.parse(event.data);
    switch (msg.type) {
        case 'workflow_saved':
            alert('Workflow saved successfully');
            break;
        case 'execution_result':
            console.log('Execution result:', msg.payload);
            break;
    }
}

function addNode(type) {
    const nodeData = {
        name: `${type}_${Date.now()}`,
        type: type,
        config: {}
    };
    editor.addNode(type, 1, 1, 100, 100, type, nodeData, type);
}

function saveWorkflow() {
    const exportData = editor.export();
    const workflow = convertToWorkflow(exportData);
    ws.send(JSON.stringify({
        type: 'save_workflow',
        payload: workflow
    }));
}

function executeWorkflow() {
    const exportData = editor.export();
    const workflow = convertToWorkflow(exportData);
    ws.send(JSON.stringify({
        type: 'execute',
        payload: workflow
    }));
}

function convertToWorkflow(exportData) {
    const nodes = {};
    const connections = exportData.drawflow.Home.data;

    // Convert nodes
    Object.entries(connections).forEach(([id, node]) => {
        nodes[id] = {
            id: id,
            type: node.class,
            name: node.data.name,
            config: node.data.config,
            children: []
        };
    });

    // Build connections
    Object.entries(connections).forEach(([id, node]) => {
        Object.entries(node.outputs).forEach(([outputName, output]) => {
            output.connections.forEach(conn => {
                nodes[id].children.push(conn.node);
            });
        });
    });

    // Find root node (node with no incoming connections)
    const root = Object.keys(nodes).find(id => {
        return !Object.values(connections).some(node =>
            Object.values(node.outputs).some(output =>
                output.connections.some(conn => conn.node === id)
            )
        );
    });

    return {
        nodes: nodes,
        root: root
    };
}

// Node templates
function sequentialTemplate() {
    return `
    <div class="node">
        <div class="node-header">Sequential</div>
        <input type="text" df-name placeholder="Handler Name">
    </div>
`;
}

function mustTemplate() {
    return `
    <div class="node">
        <div class="node-header">Must</div>
        <input type="text" df-name placeholder="Handler Name">
    </div>
`;
}