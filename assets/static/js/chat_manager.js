/**
 * Modern Chat Interface Manager
 * Modular, maintainable chat system with WebSocket streaming
 */
class ChatManager {
    constructor() {
        this.appointmentId = null;
        this.webSocket = null;
        this.isConnected = false;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 5;
        this.reconnectInterval = 3000;
        this.debugMode = false; // Set by server response
        
        // Streaming state
        this.currentStreamingMessage = '';
        this.streamingMessageId = null;
        this.typingIndicatorId = null;
        
        // DOM elements cache
        this.elements = {};
        
        // Initialize on DOM ready
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', () => this.init());
        } else {
            this.init();
        }
    }
    
    init() {
        // Get appointment ID from URL
        const urlParams = new URLSearchParams(window.location.search);
        this.appointmentId = urlParams.get('appointment_id');
        
        if (!this.appointmentId) {
            window.location.href = '/onboarding';
            return;
        }
        
        // Cache DOM elements
        this.cacheElements();
        
        // Set up event listeners
        this.setupEventListeners();
        
        // Initialize UI
        this.initializeUI();
        
        // Load chat history
        this.loadChatHistory();
        
        // Connect WebSocket
        this.connectWebSocket();
        
        // Add initial message if needed
        this.addInitialMessageIfNeeded();
    }
    
    cacheElements() {
        this.elements = {
            chatInput: document.getElementById('chat-input'),
            sendButton: document.getElementById('send-message-btn'),
            endChatButton: document.getElementById('end-chat-btn'),
            messagesContainer: document.getElementById('chat-messages'),
            statusElement: document.getElementById('chat-status'),
            statusIcon: document.getElementById('chat-status-icon'),
            sessionInfo: document.getElementById('chat-session-info'),
            sessionId: document.getElementById('session-appointment-id')
        };
        
        // Validate required elements
        const requiredElements = ['chatInput', 'sendButton', 'messagesContainer', 'statusIcon'];
        for (const key of requiredElements) {
            if (!this.elements[key]) {
                console.error(`Required element '${key}' not found`);
            }
        }
    }
    
    setupEventListeners() {
        // Send message on Enter key
        this.elements.chatInput?.addEventListener('keypress', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                this.sendMessage();
            }
        });
        
        // Send button click
        this.elements.sendButton?.addEventListener('click', () => this.sendMessage());
        
        // End chat button
        this.elements.endChatButton?.addEventListener('click', () => this.endChat());
        
        // Page unload cleanup
        window.addEventListener('beforeunload', () => this.cleanup());
        
        // Focus management
        this.elements.chatInput?.addEventListener('focus', () => this.scrollToBottom());
        
        // Input typing detection for future typing indicators
        this.elements.chatInput?.addEventListener('input', () => this.handleTyping());
    }
    
    initializeUI() {
        // Update session info
        if (this.elements.sessionId) {
            this.elements.sessionId.textContent = this.appointmentId;
        }
        if (this.elements.sessionInfo) {
            this.elements.sessionInfo.style.display = 'block';
        }
        
        // Set initial status
        this.updateStatus('Connecting...', 'info');
        
        // Disable inputs initially
        this.setInputState(false);
    }
    
    async loadChatHistory() {
        try {
            const response = await fetch(`/api/chat/history?appointment_id=${this.appointmentId}`);
            if (response.ok) {
                const data = await response.json();
                const messages = data.messages || [];
                
                // Set debug mode from server response
                this.debugMode = data.debug_mode || false;
                
                // Clear existing messages
                this.clearMessages();
                
                // Add messages to UI with animation
                for (const message of messages) {
                    this.addMessageToUI(message.role, message.content, false, true);
                }
                
                this.scrollToBottom();
            } else {
                console.error('Failed to load chat history:', response.statusText);
                this.updateStatus('Failed to load chat history', 'error');
            }
        } catch (error) {
            console.error('Error loading chat history:', error);
            this.updateStatus('Error loading chat history', 'error');
        }
    }
    
    connectWebSocket() {
        if (this.webSocket && this.webSocket.readyState === WebSocket.OPEN) {
            return; // Already connected
        }
        
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/chat/ws?appointment_id=${this.appointmentId}`;
        
        this.webSocket = new WebSocket(wsUrl);
        
        this.webSocket.onopen = (event) => {
            console.log('WebSocket connected');
            this.isConnected = true;
            this.reconnectAttempts = 0;
            this.updateStatus('Connected', 'success');
            this.setInputState(true);
            this.elements.chatInput?.focus();
        };
        
        this.webSocket.onmessage = (event) => {
            const data = JSON.parse(event.data);
            this.handleWebSocketMessage(data);
        };
        
        this.webSocket.onerror = (error) => {
            console.error('WebSocket error:', error);
            this.updateStatus('Connection error', 'error');
            this.isConnected = false;
        };
        
        this.webSocket.onclose = (event) => {
            console.log('WebSocket closed:', event.code, event.reason);
            this.isConnected = false;
            this.updateStatus('Disconnected', 'error');
            this.setInputState(false);
            
            // Try to reconnect if not a normal closure
            if (event.code !== 1000 && this.reconnectAttempts < this.maxReconnectAttempts) {
                this.reconnectAttempts++;
                const delay = this.reconnectInterval * this.reconnectAttempts;
                this.updateStatus(`Reconnecting... (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`, 'info');
                
                setTimeout(() => {
                    console.log(`Reconnect attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts}`);
                    this.connectWebSocket();
                }, delay);
            } else if (this.reconnectAttempts >= this.maxReconnectAttempts) {
                this.updateStatus('Connection failed. Please refresh the page.', 'error');
            }
        };
    }
    
    sendMessage() {
        const message = this.elements.chatInput?.value.trim();
        
        if (!message || !this.isConnected || !this.webSocket) return;
        
        // Add user message to UI with animation
        this.addMessageToUI('user', message, false, false);
        this.elements.chatInput.value = '';
        this.scrollToBottom();
        
        // Disable input while sending
        this.setInputState(false, 'Sending...');
        
        // Add typing indicator
        this.showTypingIndicator();
        
        try {
            const wsMessage = {
                type: 'user_message',
                content: message
            };
            this.webSocket.send(JSON.stringify(wsMessage));
        } catch (error) {
            console.error('Error sending message:', error);
            this.updateStatus('Error sending message', 'error');
            this.setInputState(true);
            this.hideTypingIndicator();
        }
    }
    
    handleWebSocketMessage(data) {
        console.log('WebSocket message:', data);
        
        switch (data.type) {
            case 'connected':
                this.updateStatus('Ready to chat', 'success');
                break;
                
            case 'content':
                // Remove typing indicator on first content
                if (!this.streamingMessageId) {
                    this.hideTypingIndicator();
                    // Start new streaming message
                    this.currentStreamingMessage = data.content;
                    this.streamingMessageId = this.addMessageToUI('assistant', this.currentStreamingMessage, false, false);
                } else {
                    // Append to existing streaming message
                    this.currentStreamingMessage += data.content;
                    const messageElement = document.getElementById(this.streamingMessageId);
                    if (messageElement) {
                        messageElement.textContent = this.currentStreamingMessage;
                    }
                }
                this.scrollToBottom();
                break;
                
            case 'tool_response':
                // Handle tool response and execute UI actions
                this.handleToolResponse(data.tool_response);
                break;
                
            case 'done':
                // Streaming complete
                this.currentStreamingMessage = '';
                this.streamingMessageId = null;
                this.setInputState(true);
                this.elements.chatInput?.focus();
                this.updateStatus('', '');
                break;
                
            case 'error':
                console.error('WebSocket error response:', data.error);
                this.updateStatus('Error: ' + data.error, 'error');
                this.hideTypingIndicator();
                this.setInputState(true);
                break;
                
            default:
                console.warn('Unknown WebSocket message type:', data.type);
        }
    }
    
    showTypingIndicator() {
        if (this.typingIndicatorId) return; // Already showing
        
        this.typingIndicatorId = this.addMessageToUI('assistant', 'Shirely is typing', true, false);
        
        // Add loading dots animation to typing indicator
        const typingElement = document.getElementById(this.typingIndicatorId);
        if (typingElement) {
            const dotsSpan = document.createElement('span');
            dotsSpan.className = 'loading-dots';
            typingElement.appendChild(dotsSpan);
        }
    }
    
    hideTypingIndicator() {
        if (this.typingIndicatorId) {
            const typingElement = document.getElementById(this.typingIndicatorId);
            if (typingElement) {
                typingElement.remove();
            }
            this.typingIndicatorId = null;
        }
    }
    
    addMessageToUI(role, content, isTyping = false, isHistoryLoad = false) {
        const messageDiv = document.createElement('div');
        const messageId = 'msg-' + Date.now() + '-' + Math.random().toString(36).substr(2, 9);
        
        messageDiv.id = messageId;
        messageDiv.className = `message ${role}`;
        if (isTyping) {
            messageDiv.className += ' typing';
        }
        
        // Check if this is a debug message - server already filtered them if user is not super user
        const isDebugMessage = role === 'tool' || role === 'system' || 
                              (role === 'assistant' && content.startsWith('TOOL_CALLS:'));
        
        if (isDebugMessage) {
            // Only add debug styling if we're in debug mode (super user)
            if (this.debugMode) {
                messageDiv.className += ' debug-message';
                
                // Add a prefix based on the role
                let prefix = '';
                let displayContent = content;
                
                if (role === 'assistant' && content.startsWith('TOOL_CALLS:')) {
                    prefix = '🔧 Tool Call: ';
                    // Try to parse and format the JSON for better readability
                    try {
                        const jsonStr = content.substring('TOOL_CALLS:'.length);
                        const toolCalls = JSON.parse(jsonStr);
                        displayContent = toolCalls.map(tc => `${tc.function.name}(${tc.function.arguments})`).join(', ');
                    } catch (e) {
                        // Fallback to original content
                        displayContent = content;
                    }
                } else if (role === 'tool') {
                    prefix = '⚙️ Tool Response: ';
                    // Try to parse and format the JSON for better readability
                    if (content.startsWith('TOOL_RESPONSE:')) {
                        try {
                            const jsonStr = content.substring('TOOL_RESPONSE:'.length);
                            const toolData = JSON.parse(jsonStr);
                            displayContent = `${toolData.content}`;
                        } catch (e) {
                            // Fallback to original content
                            displayContent = content;
                        }
                    }
                } else if (role === 'system') {
                    prefix = '🔧 System: ';
                }
                
                messageDiv.innerHTML = `<span class="debug-prefix">${prefix}</span><span class="debug-content">${displayContent}</span>`;
            } else {
                // This shouldn't happen since server filters these, but just in case
                return null;
            }
        } else {
            messageDiv.textContent = content;
        }
        
        // Add smooth animation for new messages
        if (!isHistoryLoad) {
            messageDiv.style.opacity = '0';
            messageDiv.style.transform = 'translateY(20px)';
        }
        
        this.elements.messagesContainer?.appendChild(messageDiv);
        
        // Trigger animation for new messages
        if (!isHistoryLoad) {
            requestAnimationFrame(() => {
                messageDiv.style.transition = 'opacity 0.3s ease, transform 0.3s ease';
                messageDiv.style.opacity = '1';
                messageDiv.style.transform = 'translateY(0)';
            });
        }
        
        this.scrollToBottom();
        return messageId;
    }
    
    addInitialMessageIfNeeded() {
        if (this.elements.messagesContainer?.children.length > 0) {
            return; // Already have messages
        }
        
        const welcomeMessage = "Hi! I'm Shirely. I'm excited to learn about your career preferences. What kind of work environment do you prefer?";
        this.addMessageToUI('assistant', welcomeMessage, false, false);
    }
    
    setInputState(enabled, statusText = '') {
        if (this.elements.chatInput) {
            this.elements.chatInput.disabled = !enabled;
        }
        if (this.elements.sendButton) {
            this.elements.sendButton.disabled = !enabled;
        }
        
        if (statusText) {
            this.updateStatus(statusText, enabled ? 'success' : 'info');
        }
    }
    
    updateStatus(message, type = '') {
        // Update the hidden text status element for fallback
        if (this.elements.statusElement) {
            this.elements.statusElement.textContent = message;
            this.elements.statusElement.className = `chat-page-status ${type}`;
        }
        
        // Update the status icon
        if (this.elements.statusIcon) {
            // Remove all status classes
            this.elements.statusIcon.className = 'chat-status-icon';
            
            // Map status types to icon classes and set tooltip
            let iconClass = '';
            let tooltip = message;
            
            switch (type) {
                case 'success':
                    if (message.includes('Connected') || message.includes('Ready')) {
                        iconClass = 'connected';
                        tooltip = 'Connected and ready';
                    }
                    break;
                case 'info':
                    if (message.includes('Connecting')) {
                        iconClass = 'connecting';
                        tooltip = 'Connecting...';
                    } else if (message.includes('Sending')) {
                        iconClass = 'sending';
                        tooltip = 'Sending message...';
                    } else if (message.includes('Reconnecting')) {
                        iconClass = 'connecting';
                        tooltip = message;
                    }
                    break;
                case 'error':
                    iconClass = 'error';
                    tooltip = message || 'Connection error';
                    break;
                default:
                    // Hide icon for empty or unknown status
                    if (!message) {
                        iconClass = '';
                        tooltip = '';
                    }
            }
            
            if (iconClass) {
                this.elements.statusIcon.className = `chat-status-icon ${iconClass}`;
            }
            
            this.elements.statusIcon.setAttribute('data-tooltip', tooltip);
        }
    }
    
    scrollToBottom() {
        if (this.elements.messagesContainer) {
            this.elements.messagesContainer.scrollTop = this.elements.messagesContainer.scrollHeight;
        }
    }
    
    clearMessages() {
        if (this.elements.messagesContainer) {
            this.elements.messagesContainer.innerHTML = '';
        }
    }
    
    handleTyping() {
        // Future: Implement typing indicators to server
        // For now, just a placeholder for the typing detection
    }
    
    /**
     * Handle tool response from the server
     * @param {Object} toolResponse - The tool response data
     */
    handleToolResponse(toolResponse) {
        console.log('Tool response received:', toolResponse);
        
        // Add tool response as a system message if it has content
        if (toolResponse.content) {
            this.addMessageToUI('system', `🔧 ${toolResponse.content}`, false, false);
        }
        
        // Execute UI actions if present
        if (toolResponse.ui_actions && Array.isArray(toolResponse.ui_actions)) {
            for (const action of toolResponse.ui_actions) {
                this.executeUIAction(action);
            }
        }
        
        // Log tool execution for debugging
        console.log(`Tool "${toolResponse.name}" executed:`, {
            success: toolResponse.success,
            content: toolResponse.content,
            ui_actions: toolResponse.ui_actions
        });
    }
    
    /**
     * Execute a UI action with optional delay
     * @param {Object} action - The UI action to execute
     */
    executeUIAction(action) {
        const delay = action.delay || 0;
        
        setTimeout(() => {
            switch (action.type) {
                case 'show_message':
                    this.showMessage(action.data.message, action.data.type || 'info');
                    break;
                    
                case 'redirect':
                    window.location.href = action.data.url;
                    break;
                    
                case 'update_status':
                    this.updateStatus(action.data.message || '', action.data.type || '');
                    break;
                    
                case 'disable_input':
                    this.setInputState(false, action.data.message || 'Chat completed');
                    break;
                    
                case 'show_completion':
                    this.showChatCompletion(action.data);
                    break;
                    
                default:
                    console.warn('Unknown UI action type:', action.type);
            }
        }, delay);
    }
    
    /**
     * Show a message to the user (using browser notification or status)
     * @param {string} message - The message to show
     * @param {string} type - The type of message (success, error, info, warning)
     */
    showMessage(message, type = 'info') {
        // For now, we'll use the status area to show messages
        // In the future, this could use toast notifications or other UI elements
        this.updateStatus(message, type);
        
        // Auto-clear the message after a few seconds
        setTimeout(() => {
            this.updateStatus('', '');
        }, 5000);
    }
    
    /**
     * Show chat completion UI
     * @param {Object} data - Completion data
     */
    showChatCompletion(data) {
        // Disable input
        this.setInputState(false, 'Chat session completed');
        
        // Add a completion message
        const completionMessage = data.message || 'Thank you for chatting with me! Your preferences have been saved.';
        this.addMessageToUI('system', `✅ ${completionMessage}`, false, false);
        
        // Show next steps if provided
        if (data.next_steps) {
            this.addMessageToUI('system', `📋 Next steps: ${data.next_steps}`, false, false);
        }
        
        this.scrollToBottom();
    }
    
    async endChat() {
        if (!this.appointmentId) return;
        
        try {
            // Close WebSocket connection
            this.cleanup();
            
            const response = await fetch(`/chat/end?appointment_id=${this.appointmentId}`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-CSRF-Token': this.getCSRFToken()
                },
                body: JSON.stringify({
                    csrf_token: this.getCSRFToken()
                })
            });
            
            if (response.ok) {
                window.location.href = '/onboarding';
            } else {
                console.error('Failed to end chat:', response.statusText);
                this.updateStatus('Failed to end chat', 'error');
            }
        } catch (error) {
            console.error('Error ending chat:', error);
            this.updateStatus('Error ending chat', 'error');
        }
    }
    
    cleanup() {
        if (this.webSocket && this.webSocket.readyState === WebSocket.OPEN) {
            this.webSocket.close(1000, 'Chat ended by user');
        }
        this.isConnected = false;
    }
    
    getCSRFToken() {
        const meta = document.querySelector('meta[name="csrf-token"]');
        return meta ? meta.getAttribute('content') : '';
    }
}

// Fast forward function for testing (admin only)
function fastForwardChat() {
    if (!window.chatManager || !window.chatManager.appointmentId) {
        alert('No appointment ID available');
        return;
    }
    
    // Disable the button during processing
    const btn = document.getElementById('fast-forward-btn');
    const originalText = btn.textContent;
    btn.disabled = true;
    btn.textContent = '⏳ Processing...';
    
    // Send request to fast-forward endpoint
    fetch(`/chat/fast-forward?appointment_id=${window.chatManager.appointmentId}`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'X-CSRF-Token': window.chatManager.getCSRFToken()
        },
        body: JSON.stringify({
            csrf_token: window.chatManager.getCSRFToken()
        })
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            // Reload chat history to show injected messages
            window.chatManager.loadChatHistory();
        } else {
            alert('Fast-forward failed: ' + (data.error || 'Unknown error'));
        }
    })
    .catch(error => {
        console.error('Fast-forward error:', error);
        alert('Fast-forward failed: ' + error.message);
    })
    .finally(() => {
        // Re-enable the button
        btn.disabled = false;
        btn.textContent = originalText;
    });
} 