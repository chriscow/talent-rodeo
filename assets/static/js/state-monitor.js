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
      
      // Show error notification using Notyf
      const notyf = new Notyf({
        duration: 0,  // Don't auto-hide
        dismissible: true,  // Allow manual dismissal
        position: { x: 'right', y: 'bottom' }
      });
      
      notyf.error({
        message: errorMsg,
        background: '#dc3545',  // Bootstrap danger color
        dismissible: true
      });

      // Redirect after a delay
      setTimeout(() => {
        window.location.href = '/profile/resume/upload';
      }, 5000);
    }
  });
});