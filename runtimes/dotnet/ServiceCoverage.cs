namespace ProtoContract;

/// <summary>Startup check requiring every registered service to be protected or explicitly exempt.</summary>
public static class ServiceCoverage
{
    public static void Validate(IEnumerable<string> registered, IEnumerable<string> protectedServices, IEnumerable<string>? exemptions = null)
    {
        var covered = new HashSet<string>(protectedServices, StringComparer.Ordinal);
        if (exemptions is not null) covered.UnionWith(exemptions);
        var missing = registered.Where(service => !covered.Contains(service)).Distinct(StringComparer.Ordinal).Order(StringComparer.Ordinal).ToArray();
        if (missing.Length != 0) throw new InvalidOperationException("contract service coverage incomplete: unprotected=" + string.Join(",", missing));
    }
}
