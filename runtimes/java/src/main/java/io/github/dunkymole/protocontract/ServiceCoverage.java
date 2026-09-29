package io.github.dunkymole.protocontract;

import java.util.Collection;
import java.util.HashSet;
import java.util.Set;

/** Startup check requiring every registered service to be protected or explicitly exempt. */
public final class ServiceCoverage {
  private ServiceCoverage() {}
  public static void validate(Collection<String> registered, Collection<String> protectedServices, Collection<String> exemptions) {
    Set<String> covered = new HashSet<>(protectedServices);
    covered.addAll(exemptions);
    Set<String> missing = new HashSet<>(registered);
    missing.removeAll(covered);
    if (!missing.isEmpty()) throw new IllegalStateException("contract service coverage incomplete: unprotected=" + missing);
  }
}
