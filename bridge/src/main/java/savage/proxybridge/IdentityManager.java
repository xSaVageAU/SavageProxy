package savage.proxybridge;

import com.mojang.authlib.GameProfile;
import com.mojang.authlib.properties.PropertyMap;
import net.minecraft.network.Connection;
import net.minecraft.server.network.ServerLoginPacketListenerImpl;
import savage.proxybridge.mixin.ConnectionAccessor;
import savage.proxybridge.mixin.ServerLoginNetworkHandlerAccessor;

import java.net.InetAddress;
import java.net.InetSocketAddress;

public class IdentityManager {

    /**
     * Injects the verified proxy identity AND remote address into the Minecraft networking stack.
     * @param handler The target login network handler.
     * @param data The verified profile and network data from the proxy.
     */
    public static void injectIdentity(ServerLoginPacketListenerImpl handler, ProfileForwardingData data) {
        // 1. Inject Identity (GameProfile)
        PropertyMap authlibProperties = new PropertyMap(data.properties());
        GameProfile profile = new GameProfile(data.uuid(), data.name(), authlibProperties);
        ((ServerLoginNetworkHandlerAccessor) handler).setAuthenticatedProfile(profile);
        
        // 2. Inject Network Address (IP Spoofing)
        Connection connection = ((ServerLoginNetworkHandlerAccessor) handler).getConnection();
        if (connection != null) {
            try {
                String fullAddr = data.remoteAddr();
                String host = fullAddr;
                int port = 0;
                
                // Parse "IP:Port" if present
                if (fullAddr.contains(":")) {
                    int lastColon = fullAddr.lastIndexOf(":");
                    host = fullAddr.substring(0, lastColon);
                    try {
                        port = Integer.parseInt(fullAddr.substring(lastColon + 1));
                    } catch (NumberFormatException ignored) {}
                }

                // Resolve to literal IP to ensure 'resolved' state in logs
                InetAddress inetAddress = InetAddress.getByName(host);
                InetSocketAddress realAddress = new InetSocketAddress(inetAddress, port);
                
                ((ConnectionAccessor) connection).setAddress(realAddress);
                
                SavageProxyConfig.LOGGER.info("Successfully injected identity & IP for {} ({}) - Resolved: {}", 
                    data.name(), data.uuid(), realAddress);
            } catch (Exception e) {
                SavageProxyConfig.LOGGER.error("Failed to resolve real IP: {}", data.remoteAddr(), e);
            }
        }
    }
}
