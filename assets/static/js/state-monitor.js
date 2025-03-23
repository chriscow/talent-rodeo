/**
 * User State Monitor
 * 
 * This script sets up real-time notifications for user state changes.
 * It will detect if a resume is invalid during async processing and notify the user.
 */

document.addEventListener('DOMContentLoaded', function() {
  const userElement = document.querySelector('[data-user-id]');
  if (!userElement) return;
  
  const userId = userElement.dataset.userId;
  const pusherKey = userElement.dataset.pusherKey;
  const pusherCluster = userElement.dataset.pusherCluster;
  
  if (!userId || !pusherKey || !pusherCluster) {
    console.error('Missing required user data for state monitoring');
    return;
  }
  
  // Initialize Pusher
  const pusher = new Pusher(pusherKey, {
    cluster: pusherCluster,
    encrypted: true
  });
  
  // Subscribe to user's channel
  const channel = pusher.subscribe(`user-${userId}`);
  
  // Listen for state updates
  channel.bind('state-update', function(data) {
    console.log('Received state update:', data);
    
    // Check if state is error (UserStatusError = 1)
    if (parseInt(data.state) === 1) {
      const errorMsg = data.error || 'There was a problem with your resume. Please upload a new one.';
      
      // Show toast notification
      const toastElement = document.createElement('div');
      toastElement.className = 'toast show';
      toastElement.role = 'alert';
      toastElement.setAttribute('aria-live', 'assertive');
      toastElement.setAttribute('aria-atomic', 'true');
      
      toastElement.innerHTML = `
        <div class="toast-header bg-danger text-white">
          <strong class="me-auto">Resume Error</strong>
          <button type="button" class="btn-close" data-bs-dismiss="toast" aria-label="Close"></button>
        </div>
        <div class="toast-body">
          ${errorMsg}
          <div class="mt-2">
            <a href="/profile/resume/upload" class="btn btn-primary btn-sm">Upload New Resume</a>
          </div>
        </div>
      `;
      
      // Create toast container if it doesn't exist
      let toastContainer = document.querySelector('.toast-container');
      if (!toastContainer) {
        toastContainer = document.createElement('div');
        toastContainer.className = 'toast-container position-fixed bottom-0 end-0 p-3';
        document.body.appendChild(toastContainer);
      }
      
      toastContainer.appendChild(toastElement);
      
      // If using Bootstrap JS, initialize toast
      if (typeof bootstrap !== 'undefined') {
        const toast = new bootstrap.Toast(toastElement, { autohide: false });
        toast.show();
      }
      
      // Redirect after a delay unless user dismisses the toast
      const redirectTimer = setTimeout(() => {
        window.location.href = '/profile/resume/upload';
      }, 5000);
      
      // Cancel redirect if user dismisses the toast
      toastElement.querySelector('.btn-close').addEventListener('click', function() {
        clearTimeout(redirectTimer);
      });
    }
  });
});