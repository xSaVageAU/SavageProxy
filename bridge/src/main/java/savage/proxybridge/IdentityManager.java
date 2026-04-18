package savage.proxybridge;

import com.mojang.authlib.GameProfile;
import com.mojang.authlib.properties.PropertyMap;
import net.minecraft.server.network.ServerLoginPacketListenerImpl;
import savage.proxybridge.mixin.ServerLoginNetworkHandlerAccessor;

public class IdentityManager {

    /**
     * Injects the verified proxy identity into the Minecraft login handler.
     * @param handler The target login network handler.
     * @param data The verified profile data from the proxy.
     */
    public static void injectIdentity(ServerLoginPacketListenerImpl handler, ProfileForwardingData data) {
        // Wrap the properties in Authlib's PropertyMap
        PropertyMap authlibProperties = new PropertyMap(data.properties());
        
        // Create the final GameProfile
        GameProfile profile = new GameProfile(data.uuid(), data.name(), authlibProperties);
        
        // Inject via Mixin Accessor
        ((ServerLoginNetworkHandlerAccessor) handler).setAuthenticatedProfile(profile);
        
        SavageProxyConfig.LOGGER.info("Successfully injected identity for {} ({})", data.name(), data.uuid());
    }
}
