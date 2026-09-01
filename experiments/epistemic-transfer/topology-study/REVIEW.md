# Independent Review

**Reviewer endpoint:** Claude Opus 4.6  
**Review stage:** completed corpus, final analyzer, manifest, analysis, and written verdict  
**Outcome:** `VERDICT SUPPORTED`

The reviewer checked:

- topology train/test partitioning;
- centroid construction;
- schema completeness and duplicate prevention;
- exact McNemar implementation;
- fold totals and confusion-matrix arithmetic;
- open-set threshold independence;
- unknown-model exclusion from centroids;
- provider and episode provenance binding;
- whether the negative conclusion was stronger than the evidence.

No critical issue or direct leakage path was found. Accuracy arithmetic and the exact `p=0.8804` full-versus-texture comparison were independently verified.

Residual cautions:

1. The final texture and full structured signature use different information channels; this does not rescue the uniqueness claim, because the proposed Lumen mechanism still fails to outperform a simple conventional fingerprint.
2. Five paired comparisons were not multiplicity-corrected. Bonferroni removes the subsidiary significance of full-versus-final-structured and full-versus-probability, but does not affect the headline texture comparison.
3. `p=0.8804` is failure to show superiority, not proof of equivalence. Overlapping Wilson intervals support the cautious wording.
4. Pilot 1 static baselines are a separate, easier acquisition regime and must not be treated as topology-held-out comparisons.
5. Uniform feature weighting is one design choice; tuning weights on this test set would be overfitting and is intentionally not attempted.

The reviewer concluded that the predeclared falsification criteria were consistently applied and that no identified issue would change the accuracy, paired test, open-set failure, or negative verdict.

---

**Addendum (2026-09-01):** The study conclusion was narrowed after this review. The verdict and accuracy findings are unchanged. The narrowing distinguishes failure of superiority and open-set claims from failure of the identification method itself: non-texture representations (probability trajectories 64.84%, graph/state 58.59%, operator summaries 28.91%) remained above the eight-class chance rate of 12.5%. Deliberate response-texture removal was not tested by this review or the study.
