package io.github.dunkymole.protocontract;

import io.grpc.*;
import java.util.*;

/** Register each implementation and contract together, then install one global guard. */
public final class ContractServerBuilder {
  private final ServerBuilder<?> delegate;
  private final Map<String, ContractServerInterceptor> contracts = new LinkedHashMap<>();
  private final Set<String> services = new HashSet<>();
  private final Set<String> exemptions;
  private boolean built;

  public ContractServerBuilder(ServerBuilder<?> delegate, String... exemptions) {
    this.delegate = Objects.requireNonNull(delegate);
    this.exemptions = Set.of(exemptions);
  }
  public ContractServerBuilder addService(BindableService implementation, ContractServerInterceptor contract) {
    if (built) throw new IllegalStateException("server builder already built");
    ServerServiceDefinition definition = implementation.bindService();
    String name = definition.getServiceDescriptor().getName();
    if (!name.equals(contract.service)) throw new IllegalArgumentException("registration does not match contract service " + contract.service);
    if (exemptions.contains(name)) throw new IllegalArgumentException("service cannot be protected and exempt");
    if (!services.add(name)) throw new IllegalArgumentException("duplicate service " + name);
    delegate.addService(definition);
    contracts.put(name, contract);
    return this;
  }
  public Server build() {
    if (built) throw new IllegalStateException("server builder already built");
    delegate.intercept(new StrictServerInterceptor(contracts, exemptions));
    Server server = delegate.build();
    built = true;
    return server;
  }
}
