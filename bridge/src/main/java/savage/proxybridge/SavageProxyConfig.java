package savage.proxybridge;

import net.minecraft.resources.Identifier;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

public class SavageProxyConfig {
    public static final String MOD_ID = "savage-proxy-bridge";
    public static final Logger LOGGER = LoggerFactory.getLogger(MOD_ID);
    
    // Security & Networking Constants
    public static final String FORWARDING_SECRET = "savage_secret_key_2026";
    public static final Identifier FORWARDING_CHANNEL = Identifier.fromNamespaceAndPath("proxy", "player_info");
}
