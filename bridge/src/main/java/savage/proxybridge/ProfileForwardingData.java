package savage.proxybridge;

import com.mojang.authlib.properties.Property;
import com.google.common.collect.LinkedHashMultimap;
import net.minecraft.network.FriendlyByteBuf;
import java.util.UUID;

/**
 * Represents the verified forwarding data received from the proxy.
 */
public record ProfileForwardingData(
    int version,
    String remoteAddr,
    UUID uuid,
    String name,
    LinkedHashMultimap<String, Property> properties
) {
    public static ProfileForwardingData fromBuf(FriendlyByteBuf buf) {
        int version = buf.readVarInt();
        String remoteAddr = buf.readUtf();
        UUID uuid = buf.readUUID();
        String name = buf.readUtf();

        LinkedHashMultimap<String, Property> properties = LinkedHashMultimap.create();
        int propertyCount = buf.readVarInt();
        for (int i = 0; i < propertyCount; i++) {
            String propName = buf.readUtf();
            String propValue = buf.readUtf();
            String signature = buf.readBoolean() ? buf.readUtf() : null;
            properties.put(propName, new Property(propName, propValue, signature));
        }

        return new ProfileForwardingData(version, remoteAddr, uuid, name, properties);
    }
}
