import numpy as np
import matplotlib.pyplot as plt

def rrf_score(dense_score, sparse_score, k=60):
    """Calculate RRF score as implemented in your Go code"""
    return 1.0/(k + (1.0/dense_score)) + 1.0/(k + (1.0/sparse_score))

# Your actual score ranges
dense_scores = np.linspace(0.01, 1.0, 100)  # Dense: 0 to 1
sparse_scores = np.linspace(0.01, 20.0, 100)  # Sparse: 0 to 20

# Calculate RRF scores for various combinations
rrf_scores = []

print("RRF Score Analysis with your actual ranges:")
print("Dense: 0.01 to 1.0, Sparse: 0.01 to 20.0")
print("-" * 50)

# Test corner cases
test_cases = [
    (0.01, 0.01, "Minimum both"),
    (1.0, 20.0, "Maximum both"),
    (0.1, 0.1, "Low both"),
    (0.5, 5.0, "Medium both"),
    (0.9, 15.0, "High both"),
    (0.01, 20.0, "Min dense, Max sparse"),
    (1.0, 0.01, "Max dense, Min sparse"),
]

for dense, sparse, description in test_cases:
    score = rrf_score(dense, sparse)
    rrf_scores.append(score)
    print(f"{description:20}: Dense={dense:5.2f}, Sparse={sparse:5.2f} -> RRF={score:.6f}")

# Find actual min/max across realistic ranges
min_rrf = float('inf')
max_rrf = 0

for dense in [0.01, 0.1, 0.3, 0.5, 0.7, 0.9, 1.0]:
    for sparse in [0.01, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 15.0, 20.0]:
        score = rrf_score(dense, sparse)
        min_rrf = min(min_rrf, score)
        max_rrf = max(max_rrf, score)

print(f"\nActual RRF Range:")
print(f"Minimum: {min_rrf:.6f}")
print(f"Maximum: {max_rrf:.6f}")
print(f"Range: {max_rrf - min_rrf:.6f}")

# Test percentage mapping
def new_rrf_percentage(score, min_rrf, max_rrf):
    if score <= min_rrf:
        return 0
    if score >= max_rrf:
        return 100
    percentage = ((score - min_rrf) / (max_rrf - min_rrf)) * 100
    return int(round(percentage))

print(f"\nPercentage mapping examples:")
for dense, sparse, description in test_cases[:5]:
    score = rrf_score(dense, sparse)
    pct = new_rrf_percentage(score, min_rrf, max_rrf)
    print(f"{description:20}: RRF={score:.6f} -> {pct}%")