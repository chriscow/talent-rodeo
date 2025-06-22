// Onboarding Modal Sequence JavaScript

let currentModalStep = 1;
const totalSteps = 4;

document.addEventListener('DOMContentLoaded', function() {
    initializeOnboarding();
    setupModalAnimations();
    setupKeyboardNavigation();
});

function initializeOnboarding() {
    // Auto-start the onboarding sequence
    setTimeout(() => {
        showOnboardingStep(1);
    }, 500);
}

function showOnboardingStep(step) {
    if (step < 1 || step > totalSteps) return;
    
    // Close current modal if open
    if (currentModalStep !== step) {
        const currentModal = document.getElementById(`onboarding-modal-${currentModalStep}`);
        if (currentModal && currentModal.open) {
            currentModal.close();
        }
    }
    
    currentModalStep = step;
    const modal = document.getElementById(`onboarding-modal-${step}`);
    
    if (modal) {
        // Update progress bar
        updateProgressBar(step);
        
        // Show modal with animation
        modal.showModal();
        
        // Animate modal entrance
        animateModalEntrance(modal);
        
        // Setup staggered content animations
        setTimeout(() => {
            setupStaggeredAnimations(modal);
        }, 300);
    }
}

function nextOnboardingStep(step) {
    if (step <= totalSteps) {
        animateModalTransition(() => {
            showOnboardingStep(step);
        });
    }
}

function previousOnboardingStep(step) {
    if (step >= 1) {
        animateModalTransition(() => {
            showOnboardingStep(step);
        });
    }
}

function animateModalTransition(callback) {
    const currentModal = document.getElementById(`onboarding-modal-${currentModalStep}`);
    
    if (currentModal) {
        // Animate current modal exit
        currentModal.style.transform = 'scale(0.95) translateY(-10px)';
        currentModal.style.opacity = '0.7';
        
        setTimeout(() => {
            currentModal.close();
            callback();
        }, 200);
    } else {
        callback();
    }
}

function updateProgressBar(step) {
    const progress = (step / totalSteps) * 100;
    const progressBar = document.querySelector(`#onboarding-modal-${step} .progress-bar`);
    
    if (progressBar) {
        progressBar.style.setProperty('--progress', `${progress}%`);
    }
}

function animateModalEntrance(modal) {
    // Reset transform
    modal.style.transform = 'scale(1) translateY(0)';
    modal.style.opacity = '1';
    
    // Animate modal article entrance
    const article = modal.querySelector('article');
    if (article) {
        article.style.transform = 'translateY(20px)';
        article.style.opacity = '0';
        
        setTimeout(() => {
            article.style.transition = 'all 0.4s cubic-bezier(0.4, 0, 0.2, 1)';
            article.style.transform = 'translateY(0)';
            article.style.opacity = '1';
        }, 100);
    }
}

function setupStaggeredAnimations(modal) {
    // Animate feature cards with stagger
    const featureCards = modal.querySelectorAll('.feature-card');
    featureCards.forEach((card, index) => {
        card.style.opacity = '0';
        card.style.transform = 'translateY(30px)';
        
        setTimeout(() => {
            card.style.transition = 'all 0.6s cubic-bezier(0.4, 0, 0.2, 1)';
            card.style.opacity = '1';
            card.style.transform = 'translateY(0)';
        }, index * 150);
    });
    
    // Animate flow steps with stagger
    const flowSteps = modal.querySelectorAll('.flow-step');
    flowSteps.forEach((step, index) => {
        step.style.opacity = '0';
        step.style.transform = 'translateX(-50px)';
        
        setTimeout(() => {
            step.style.transition = 'all 0.6s cubic-bezier(0.4, 0, 0.2, 1)';
            step.style.opacity = '1';
            step.style.transform = 'translateX(0)';
        }, index * 200);
    });
    
    // Animate benefit items with stagger
    const benefitItems = modal.querySelectorAll('.benefit-item');
    benefitItems.forEach((item, index) => {
        item.style.opacity = '0';
        item.style.transform = 'translateX(-30px)';
        
        setTimeout(() => {
            item.style.transition = 'all 0.5s cubic-bezier(0.4, 0, 0.2, 1)';
            item.style.opacity = '1';
            item.style.transform = 'translateX(0)';
        }, index * 100);
    });
    
    // Animate hero text
    const heroText = modal.querySelector('.hero-text');
    if (heroText) {
        heroText.style.opacity = '0';
        heroText.style.transform = 'translateY(20px)';
        
        setTimeout(() => {
            heroText.style.transition = 'all 0.6s cubic-bezier(0.4, 0, 0.2, 1)';
            heroText.style.opacity = '1';
            heroText.style.transform = 'translateY(0)';
        }, 100);
    }
    
    // Animate description text
    const descText = modal.querySelector('.description-text');
    if (descText) {
        descText.style.opacity = '0';
        descText.style.transform = 'translateY(20px)';
        
        setTimeout(() => {
            descText.style.transition = 'all 0.6s cubic-bezier(0.4, 0, 0.2, 1)';
            descText.style.opacity = '1';
            descText.style.transform = 'translateY(0)';
        }, 200);
    }
}

function setupModalAnimations() {
    // Enhanced button hover effects
    const buttons = document.querySelectorAll('.modal-footer button');
    
    buttons.forEach(button => {
        button.addEventListener('mouseenter', function() {
            this.style.transform = 'translateY(-2px) scale(1.02)';
        });
        
        button.addEventListener('mouseleave', function() {
            this.style.transform = 'translateY(0) scale(1)';
        });
        
        button.addEventListener('mousedown', function() {
            this.style.transform = 'translateY(0) scale(0.98)';
        });
        
        button.addEventListener('mouseup', function() {
            this.style.transform = 'translateY(-2px) scale(1.02)';
        });
    });
    
    // Enhanced card hover effects
    const cards = document.querySelectorAll('.feature-card, .old-way, .new-way, .flow-step');
    
    cards.forEach(card => {
        card.addEventListener('mouseenter', function() {
            this.style.transform = 'translateY(-5px)';
            this.style.boxShadow = '0 12px 30px rgba(0, 0, 0, 0.15)';
        });
        
        card.addEventListener('mouseleave', function() {
            this.style.transform = 'translateY(0)';
            this.style.boxShadow = '0 4px 6px rgba(0, 0, 0, 0.1)';
        });
    });
}

function setupKeyboardNavigation() {
    document.addEventListener('keydown', function(event) {
        // Only handle keyboard navigation if a modal is open
        const openModal = document.querySelector('.onboarding-modal[open]');
        if (!openModal) return;
        
        if (event.key === 'ArrowLeft' && currentModalStep > 1) {
            event.preventDefault();
            previousOnboardingStep(currentModalStep - 1);
        } else if (event.key === 'ArrowRight' && currentModalStep < totalSteps) {
            event.preventDefault();
            nextOnboardingStep(currentModalStep + 1);
        } else if (event.key === 'Enter') {
            event.preventDefault();
            if (currentModalStep < totalSteps) {
                nextOnboardingStep(currentModalStep + 1);
            } else {
                completeOnboarding();
            }
        } else if (event.key === 'Escape') {
            event.preventDefault();
            skipOnboarding();
        }
    });
}

function skipOnboarding() {
    const currentModal = document.getElementById(`onboarding-modal-${currentModalStep}`);
    if (currentModal) {
        currentModal.close();
    }
    
    // Redirect to profile or main app
    window.location.href = '/profile';
}

function completeOnboarding() {
    const currentModal = document.getElementById(`onboarding-modal-${currentModalStep}`);
    if (currentModal) {
        // Add completion animation
        const article = currentModal.querySelector('article');
        article.style.transform = 'scale(1.05)';
        article.style.opacity = '0.8';
        
        setTimeout(() => {
            currentModal.close();
            // Redirect to profile/resume upload
            window.location.href = '/onboarding/complete';
        }, 300);
    }
}

// Prevent modal from closing when clicking inside modal content
document.addEventListener('click', function(event) {
    const modals = document.querySelectorAll('.onboarding-modal');
    modals.forEach(modal => {
        if (modal.open && event.target === modal) {
            event.preventDefault();
            // Optionally add a subtle shake animation to indicate it can't be closed
            const article = modal.querySelector('article');
            article.style.animation = 'shake 0.5s ease-in-out';
            setTimeout(() => {
                article.style.animation = '';
            }, 500);
        }
    });
});

// Add shake animation for when user tries to close modal by clicking backdrop
const style = document.createElement('style');
style.textContent = `
    @keyframes shake {
        0%, 100% { transform: translateX(0); }
        25% { transform: translateX(-5px); }
        75% { transform: translateX(5px); }
    }
`;
document.head.appendChild(style);
