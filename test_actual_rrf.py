#!/usr/bin/env python3
"""
Test the actual RRF implementation used in the Go code to understand the 0.2193 score.
"""

def actual_rrf_fusion(dense_score, sparse_score, k=60.0):
    """
    The actual RRF implementation from the Go code:
    return 1.0/(k+(1.0/denseScore)) + 1.0/(k+(1.0/sparseScore))
    """
    return 1.0/(k + (1.0/dense_score)) + 1.0/(k + (1.0/sparse_score))

def recency_boost(score, recency_weight, decay_factor):
    """
    Apply recency boosting: (1-weight)*score + weight*decay_factor
    """
    return (1.0 - recency_weight) * score + recency_weight * decay_factor

# Test various score combinations to see if we can get 0.2193
target_score = 0.2193
k = 60.0

print("Testing actual RRF implementation to find source of 0.2193 score")
print("=" * 60)

# Test high scores that might produce 0.2193
test_cases = [
    (0.95, 0.90),
    (0.98, 0.95),
    (1.0, 1.0),
    (0.9, 0.9),
    (0.99, 0.99),
    (0.8, 0.8),
    (0.85, 0.9),
    (0.92, 0.88),
]

print("Testing pure RRF scores:")
for dense, sparse in test_cases:
    rrf_score = actual_rrf_fusion(dense, sparse, k)
    print(f"Dense={dense:4.2f}, Sparse={sparse:4.2f} -> RRF={rrf_score:.6f}")
    
    # Check if close to target
    if abs(rrf_score - target_score) < 0.001:
        print(f"  *** MATCH FOUND: {rrf_score:.6f} ≈ {target_score}")

print("\nTesting RRF + recency boosting:")
print("Trying different recency weights and decay factors...")

# Test common recency configurations
recency_configs = [
    (0.2, 0.5),   # Default config
    (0.2, 1.0),   # Recent post
    (0.4, 0.5),   # Higher weight
    (0.3, 0.7),   # Medium config
]

for dense, sparse in [(0.95, 0.90), (0.98, 0.95), (0.92, 0.88)]:
    base_rrf = actual_rrf_fusion(dense, sparse, k)
    
    for weight, decay in recency_configs:
        boosted = recency_boost(base_rrf, weight, decay)
        if abs(boosted - target_score) < 0.005:
            print(f"Dense={dense:4.2f}, Sparse={sparse:4.2f}, RRF={base_rrf:.6f}")
            print(f"  + Recency(weight={weight}, decay={decay}) = {boosted:.6f}")
            print(f"  *** CLOSE MATCH: {boosted:.6f} ≈ {target_score}")

# Calculate the theoretical maximum RRF score
max_rrf = actual_rrf_fusion(1.0, 1.0, k)
print(f"\nTheoretical maximum RRF score: {max_rrf:.6f}")

# Calculate what scores would give exactly 0.2193
print(f"\nReverse engineering target score {target_score}:")
# If both scores are equal: 2 * (1/(k + 1/score)) = target
# So: 1/(k + 1/score) = target/2
# So: k + 1/score = 2/target
# So: 1/score = 2/target - k
# So: score = 1/(2/target - k)

if (2/target_score - k) > 0:
    equal_score = 1/(2/target_score - k)
    check = actual_rrf_fusion(equal_score, equal_score, k)
    print(f"If both dense and sparse = {equal_score:.6f}, RRF = {check:.6f}")
else:
    print("Target score too high for equal dense/sparse scores")
